package api

import (
	"net/http"
)

// for talking to node using hhtp
type StorageCLient struct {
	NodeAddress string

	HTTPClient *http.Client // htpp client can be reused with connection pooling
}
