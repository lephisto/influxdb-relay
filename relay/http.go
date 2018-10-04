package relay

import (
	"bytes"
	"compress/gzip"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/influxdata/influxdb/models"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	. "git.vpgrp.io/lsantoni/influxdb-relay/config"
)

// HTTP is a relay for HTTP influxdb writes
type HTTP struct {
	addr   string
	name   string
	schema string

	cert string
	rp   string

	closing int64
	l       net.Listener

	backends []*httpBackend

	start time.Time
}

type relayHandlerFunc func(h *HTTP, w http.ResponseWriter, r *http.Request)
type relayMiddleware func(h *HTTP, handlerFunc relayHandlerFunc) relayHandlerFunc

// Default HTTP settings and a few constants
const (
	DefaultHTTPTimeout      = 10 * time.Second
	DefaultMaxDelayInterval = 10 * time.Second
	DefaultBatchSizeKB      = 512

	KB = 1024
	MB = 1024 * KB
)

var handlers = map[string]relayHandlerFunc{
	"/write":             (*HTTP).handleStandard,
	"/api/v1/prom/write": (*HTTP).handleProm,
	"/ping":              (*HTTP).handlePing,
	"/status":            (*HTTP).handleStatus,
}

var middlewares = []relayMiddleware{
	(*HTTP).bodyMiddleWare,
	(*HTTP).queryMiddleWare,
}

// NewHTTP creates a new HTTP relay
// This relay will most likely be tied to a RelayService
// and manage a set of HTTPBackends
func NewHTTP(cfg HTTPConfig) (Relay, error) {
	h := new(HTTP)

	h.addr = cfg.Addr
	h.name = cfg.Name

	h.cert = cfg.SSLCombinedPem
	h.rp = cfg.DefaultRetentionPolicy

	// If a cert is specified, this means the user
	// wants to do HTTPS
	h.schema = "http"
	if h.cert != "" {
		h.schema = "https"
	}

	// For each output specified in the config, we are going to create a backend
	for i := range cfg.Outputs {
		backend, err := newHTTPBackend(&cfg.Outputs[i])
		if err != nil {
			return nil, err
		}

		h.backends = append(h.backends, backend)
	}

	return h, nil
}

// Name is the name of the HTTP relay
// a default name might be generated if it is
// not specified in the configuration file
func (h *HTTP) Name() string {
	if h.name == "" {
		return fmt.Sprintf("%s://%s", h.schema, h.addr)
	}

	return h.name
}

// Run actually launch the HTTP endpoint
func (h *HTTP) Run() error {
	l, err := net.Listen("tcp", h.addr)
	if err != nil {
		return err
	}

	// support HTTPS
	if h.cert != "" {
		cert, err := tls.LoadX509KeyPair(h.cert, h.cert)
		if err != nil {
			return err
		}

		l = tls.NewListener(l, &tls.Config{
			Certificates: []tls.Certificate{cert},
		})
	}

	h.l = l

	log.Printf("Starting %s relay %q on %v", strings.ToUpper(h.schema), h.Name(), h.addr)

	// TODO: Comment this block
	err = http.Serve(l, h)
	if atomic.LoadInt64(&h.closing) != 0 {
		return nil
	}
	return err
}

// Stop actually stops the HTTP endpoint
func (h *HTTP) Stop() error {
	atomic.StoreInt64(&h.closing, 1)
	return h.l.Close()
}

// ServeHTTP is the function that handles the different route
// The response is a JSON object describing the state of the operation
// TODO: Formalize JSON return value into a structure
// TODO: Split this into a router
// TODO: Allow different treatments based on the target "type"
func (h *HTTP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.start = time.Now()

	fmt.Println("Got request on: " + r.URL.Path)

	if fun, ok := handlers[r.URL.Path]; ok {
		allMiddlewares(h, fun)(h, w, r)
	} else {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
	}
}

func allMiddlewares(h *HTTP, handlerFunc relayHandlerFunc) relayHandlerFunc {
	var res = handlerFunc
	for _, middleware := range middlewares {
		res = middleware(h, res)
	}
	return res
}

func (h *HTTP) bodyMiddleWare(next relayHandlerFunc) relayHandlerFunc {
	return relayHandlerFunc(func(h *HTTP, w http.ResponseWriter, r *http.Request) {
		var body = r.Body

		if r.Header.Get("Content-Encoding") == "gzip" {
			b, err := gzip.NewReader(r.Body)
			if err != nil {
				jsonResponse(w, http.StatusBadRequest, "unable to decode gzip body")
			}
			defer b.Close()
			body = b
		}

		r.Body = body
		next(h, w, r)
	})
}

func (h *HTTP) queryMiddleWare(next relayHandlerFunc) relayHandlerFunc {
	return relayHandlerFunc(func(h *HTTP, w http.ResponseWriter, r *http.Request) {
		queryParams := r.URL.Query()

		if queryParams.Get("db") == "" {
			jsonResponse(w, http.StatusBadRequest, "missing parameter: db")
			return
		}

		if queryParams.Get("rp") == "" && h.rp != "" {
			queryParams.Set("rp", h.rp)
		}
		r.URL.RawQuery = queryParams.Encode()
		next(h, w, r)
	})

}

func (h *HTTP) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		st := make(map[string]map[string]string)

		for _, b := range h.backends {
			st[b.name] = b.poster.getStats()
		}

		j, err := json.Marshal(st)

		if err != nil {
			log.Printf("error: %s", err)
			jsonResponse(w, http.StatusInternalServerError, "json marshalling failed")
			return
		}

		jsonResponse(w, http.StatusOK, fmt.Sprintf("\"status\": %s", string(j)))
	} else {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	}
}

func (h *HTTP) handlePing(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		w.Header().Add("X-InfluxDB-Version", "relay")
		w.WriteHeader(http.StatusNoContent)
	} else {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	}
}

func (h *HTTP) handleStandard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
		} else {
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		}
	}

	queryParams := r.URL.Query()
	bodyBuf := getBuf()
	_, err := bodyBuf.ReadFrom(r.Body)
	if err != nil {
		putBuf(bodyBuf)
		jsonResponse(w, http.StatusInternalServerError, "problem reading request body")
		return
	}

	precision := queryParams.Get("precision")
	points, err := models.ParsePointsWithPrecision(bodyBuf.Bytes(), h.start, precision)
	if err != nil {
		putBuf(bodyBuf)
		jsonResponse(w, http.StatusBadRequest, "unable to parse points")
		return
	}

	outBuf := getBuf()
	for _, p := range points {
		if _, err = outBuf.WriteString(p.PrecisionString(precision)); err != nil {
			break
		}
		if err = outBuf.WriteByte('\n'); err != nil {
			break
		}
	}

	// done with the input points
	putBuf(bodyBuf)

	if err != nil {
		putBuf(outBuf)
		jsonResponse(w, http.StatusInternalServerError, "problem writing points")
		return
	}

	// normalize query string
	query := queryParams.Encode()

	outBytes := outBuf.Bytes()

	// check for authorization performed via the header
	authHeader := r.Header.Get("Authorization")

	var wg sync.WaitGroup
	wg.Add(len(h.backends))

	var responses = make(chan *responseData, len(h.backends))

	for _, b := range h.backends {
		b := b
		if b.inputType != TypeInfluxdb {
			continue
		}
		go func() {
			defer wg.Done()
			resp, err := b.post(outBytes, query, authHeader)
			if err != nil {
				log.Printf("Problem posting to relay %q backend %q: %v", h.Name(), b.name, err)
			} else {
				if resp.StatusCode/100 == 5 {
					log.Printf("5xx response for relay %q backend %q: %v", h.Name(), b.name, resp.StatusCode)
				}
				responses <- resp
			}
		}()
	}

	go func() {
		wg.Wait()
		close(responses)
		putBuf(outBuf)
	}()

	var errResponse *responseData

	for resp := range responses {
		switch resp.StatusCode / 100 {
		case 2:
			w.WriteHeader(http.StatusNoContent)
			return

		case 4:
			// user error
			resp.Write(w)
			return

		default:
			// hold on to one of the responses to return back to the client
			errResponse = resp
		}
	}

	// no successful writes
	if errResponse == nil {
		// failed to make any valid request...
		jsonResponse(w, http.StatusServiceUnavailable, "unable to write points")
		return
	}

	errResponse.Write(w)
}

func (h *HTTP) handleProm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
		} else {
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		}
	}

	authHeader := r.Header.Get("Authorization")

	var wg sync.WaitGroup
	wg.Add(len(h.backends))

	var responses = make(chan *responseData, len(h.backends))

	for _, b := range h.backends {
		b := b
		if b.inputType != TypePrometheus {
			continue
		}
		go func() {
			defer wg.Done()
			content, _ := ioutil.ReadAll(r.Body)
			resp, err := b.post(content, r.URL.RawQuery, authHeader)
			if err != nil {
				log.Printf("Problem posting to relay %q backend %q: %v", h.Name(), b.name, err)
			} else {
				if resp.StatusCode/100 == 5 {
					log.Printf("5xx response for relay %q backend %q: %v", h.Name(), b.name, resp.StatusCode)
				}
				responses <- resp
			}
		}()
	}

	go func() {
		wg.Wait()
		close(responses)
	}()

	var errResponse *responseData

	for resp := range responses {
		switch resp.StatusCode / 100 {
		case 2:
			w.WriteHeader(http.StatusNoContent)
			return

		case 4:
			// user error
			resp.Write(w)
			return

		default:
			// hold on to one of the responses to return back to the client
			errResponse = resp
		}
	}

	// no successful writes
	if errResponse == nil {
		// failed to make any valid request...
		jsonResponse(w, http.StatusServiceUnavailable, "unable to write points")
		return
	}

	errResponse.Write(w)
}

type responseData struct {
	ContentType     string
	ContentEncoding string
	StatusCode      int
	Body            []byte
}

func (rd *responseData) Write(w http.ResponseWriter) {
	if rd.ContentType != "" {
		w.Header().Set("Content-Type", rd.ContentType)
	}

	if rd.ContentEncoding != "" {
		w.Header().Set("Content-Encoding", rd.ContentEncoding)
	}

	w.Header().Set("Content-Length", strconv.Itoa(len(rd.Body)))
	w.WriteHeader(rd.StatusCode)
	w.Write(rd.Body)
}

func jsonResponse(w http.ResponseWriter, code int, message string) {
	var data string
	if code/100 != 2 {
		data = fmt.Sprintf("{\"error\":%q}\n", message)
	} else {
		data = fmt.Sprintf("{%s}\n", message)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", fmt.Sprint(len(data)))
	w.WriteHeader(code)
	w.Write([]byte(data))
}

type poster interface {
	post([]byte, string, string) (*responseData, error)
	getStats() map[string]string
}

type simplePoster struct {
	client   *http.Client
	location string
}

func newSimplePoster(location string, timeout time.Duration, skipTLSVerification bool) *simplePoster {
	// Configure custom transport for http.Client
	// Used for support skip-tls-verification option
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: skipTLSVerification,
		},
	}

	return &simplePoster{
		client: &http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
		location: location,
	}
}

func (s *simplePoster) getStats() map[string]string {
	v := make(map[string]string)
	v["location"] = s.location
	return v
}

func (b *simplePoster) post(buf []byte, query string, auth string) (*responseData, error) {
	req, err := http.NewRequest("POST", b.location, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}

	req.URL.RawQuery = query
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("Content-Length", strconv.Itoa(len(buf)))
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return &responseData{
		ContentType:     resp.Header.Get("Content-Type"),
		ContentEncoding: resp.Header.Get("Content-Encoding"),
		StatusCode:      resp.StatusCode,
		Body:            data,
	}, nil
}

type httpBackend struct {
	poster
	name      string
	inputType Input
}

func newHTTPBackend(cfg *HTTPOutputConfig) (*httpBackend, error) {
	// Get default name
	if cfg.Name == "" {
		cfg.Name = cfg.Location
	}

	timeout := DefaultHTTPTimeout
	if cfg.Timeout != "" {
		t, err := time.ParseDuration(cfg.Timeout)
		if err != nil {
			return nil, fmt.Errorf("error parsing HTTP timeout '%v'", err)
		}
		timeout = t
	}

	var p poster = newSimplePoster(cfg.Location, timeout, cfg.SkipTLSVerification)

	// If configured, create a retryBuffer per backend.
	// This way we serialize retries against each backend.
	if cfg.BufferSizeMB > 0 {
		max := DefaultMaxDelayInterval
		if cfg.MaxDelayInterval != "" {
			m, err := time.ParseDuration(cfg.MaxDelayInterval)
			if err != nil {
				return nil, fmt.Errorf("error parsing max retry time %v", err)
			}
			max = m
		}

		batch := DefaultBatchSizeKB * KB
		if cfg.MaxBatchKB > 0 {
			batch = cfg.MaxBatchKB * KB
		}

		p = newRetryBuffer(cfg.BufferSizeMB*MB, batch, max, p)
	}

	return &httpBackend{
		poster:    p,
		name:      cfg.Name,
		inputType: cfg.InputType,
	}, nil
}

// ErrBufferFull -TODO-
var ErrBufferFull = errors.New("retry buffer full")

var bufPool = sync.Pool{New: func() interface{} { return new(bytes.Buffer) }}

func getBuf() *bytes.Buffer {
	if bb, ok := bufPool.Get().(*bytes.Buffer); ok {
		return bb
	}
	return new(bytes.Buffer)
}

func putBuf(b *bytes.Buffer) {
	b.Reset()
	bufPool.Put(b)
}
