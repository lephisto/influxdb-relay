package relay

import (
	"encoding/json"
	"fmt"
	"github.com/influxdata/influxdb/models"
	"github.com/vente-privee/influxdb-relay/config"
	"io/ioutil"
	"log"
	"net/http"
	"sync"
)

func (h *HTTP) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		st := make(map[string]map[string]string)

		for _, b := range h.backends {
			st[b.name] = b.poster.getStats()
		}

		j, _ := json.Marshal(st)

		jsonResponse(w, response{http.StatusOK, fmt.Sprintf("\"status\": %s", string(j))})
	} else {
		jsonResponse(w, response{http.StatusMethodNotAllowed, http.StatusText(http.StatusMethodNotAllowed)})
		return
	}
}

func (h *HTTP) handlePing(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		for key, value := range h.pingResponseHeaders {
			w.Header().Add(key, value)
		}
		w.WriteHeader(http.StatusNoContent)
	} else {
		jsonResponse(w, response{http.StatusMethodNotAllowed, http.StatusText(http.StatusMethodNotAllowed)})
		return
	}
}

func (h *HTTP) handleStandard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
		} else {
			jsonResponse(w, response{http.StatusMethodNotAllowed, http.StatusText(http.StatusMethodNotAllowed)})
			return
		}
	}

	queryParams := r.URL.Query()
	bodyBuf := getBuf()
	_, err := bodyBuf.ReadFrom(r.Body)
	if err != nil {
		putBuf(bodyBuf)
		jsonResponse(w, response{http.StatusInternalServerError, "problem reading request body"})
		return
	}

	precision := queryParams.Get("precision")
	points, err := models.ParsePointsWithPrecision(bodyBuf.Bytes(), h.start, precision)
	if err != nil {
		putBuf(bodyBuf)
		jsonResponse(w, response{http.StatusBadRequest, "unable to parse points"})
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
		jsonResponse(w, response{http.StatusInternalServerError, "problem writing points"})
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
		if b.inputType != config.TypeInfluxdb {
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
		jsonResponse(w, response{http.StatusServiceUnavailable, "unable to write points"})
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
			jsonResponse(w, response{http.StatusMethodNotAllowed, http.StatusText(http.StatusMethodNotAllowed)})
			return
		}
	}

	authHeader := r.Header.Get("Authorization")

	var wg sync.WaitGroup
	wg.Add(len(h.backends))

	var responses = make(chan *responseData, len(h.backends))

	for _, b := range h.backends {
		b := b
		if b.inputType != config.TypePrometheus {
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
		jsonResponse(w, response{http.StatusServiceUnavailable, "unable to write points"})
		return
	}

	errResponse.Write(w)
}
