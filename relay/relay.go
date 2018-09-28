package relay

// Relay is an HTTP or UDP endpoint
type Relay interface {
	Name() string
	Run() error
	Stop() error
}
