package reporter

import (
	"fmt"
	"net/http"
	"strings"
)

type ExplorerError struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
}

func (e ExplorerError) Error() string {
	return fmt.Sprintf("%d: %s", e.Code, e.Message)
}

//NoBlockFound returns true if it's a no block found error
func (e ExplorerError) NoBlockFound() bool {
	return e.Code == http.StatusBadRequest && strings.HasPrefix(e.Message, "no block found")
}
