package reporter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"time"
)

//CoinOutput struct
type CoinOutput struct {
	Value      json.Number `json:"value"`
	UnlockHash string      `json:"unlockhash"`
}

//RawTransaction struct
type RawTransaction struct {
	Version int `jons:"version"`
	Data    struct {
		CoinOutputs []CoinOutput  `json:"coinoutputs"`
		MinerFees   []json.Number `json:"minerfees"`
	} `json:"data"`
}

//InputOutput struct
type InputOutput struct {
	Value      json.Number `json:"value"`
	UnlockHash string      `json:"unlockhash"`
	//condition (we ignore for now)
}

//Transaction struct
type Transaction struct {
	ID     string `json:"id"`
	Height int64  `json:"height"`
	Parent string `json:"parent"`

	RawTransaction   RawTransaction `json:"rawtransaction"`
	CoinInputOutputs []InputOutput  `json:"coininputoutputs"`
	//CoinOutputUnlockHashes []string       `json:"coinoutputunlockhashes"`

	// BlockStakeOutputIDs    []string      `json:"blockstakeoutputids"`
	// BlockStakeUnlockHashes []string      `json:"blockstakeunlockhashes"`
}

//Block struct
type Block struct {
	Transactions []Transaction `json:"transactions"`
	Height       int64         `json:"height"`

	RawBlock struct {
		Timestamp int64 `json:"timestamp"`
	} `json:"rawblock"`
}

//Explorer an explorer client interface
type Explorer interface {
	GetBlock(h int64) (*Block, error)
	Scan(h int64) Scanner
}

//Scanner as explorer scanner
type Scanner interface {
	Scan(ctx context.Context) <-chan *Block
	Err() error
}

//NewExplorer creates a new explorer client
func NewExplorer(u string) (Explorer, error) {
	cl := &http.Client{}
	uri, err := url.Parse(u)
	if err != nil {
		return nil, err
	}
	if uri.Scheme != "http" && uri.Scheme != "https" {
		return nil, fmt.Errorf("invalid url scheme")
	}

	return &httpExplorer{u: uri, cl: cl}, nil
}

const (
	blockEndpoint = "explorer/blocks/"
)

type httpExplorer struct {
	cl *http.Client
	u  *url.URL
}

func (e *httpExplorer) errorFromResponse(r *http.Response) error {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}

	var o ExplorerError
	if err := json.Unmarshal(body, &o); err != nil {
		return fmt.Errorf("failed to parse error message '%v': %v", string(body), err)
	}

	o.Code = r.StatusCode
	return o
}

func (e *httpExplorer) request(method, endpoint string, body io.Reader) (*http.Request, error) {
	url := e.u.String() + "/" + endpoint
	request, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}

	//todo: we need to make the agent customizable
	request.Header.Set("user-agent", "Rivine-Agent")

	return request, nil
}

func (e *httpExplorer) GetBlock(h int64) (*Block, error) {
	request, err := e.request(http.MethodGet, path.Join(blockEndpoint, fmt.Sprint(h)), nil)
	if err != nil {
		return nil, err
	}

	response, err := e.cl.Do(request)
	if err != nil {
		return nil, err
	}

	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, e.errorFromResponse(response)
	}

	enc := json.NewDecoder(response.Body)
	var body struct {
		Block Block `json:"block"`
	}
	if err := enc.Decode(&body); err != nil {
		return nil, err
	}

	return &body.Block, nil
}

func (e *httpExplorer) Scan(head int64) Scanner {
	return &explorerScanner{exp: e, head: head}
}

type explorerScanner struct {
	exp  Explorer
	head int64
	err  error
}

func (s *explorerScanner) Err() error {
	return s.err
}

func (s *explorerScanner) Scan(ctx context.Context) <-chan *Block {
	ch := make(chan *Block)

	go func() {
		defer close(ch)

		for {
			blk, err := s.exp.GetBlock(s.head)
			switch err := err.(type) {
			case ExplorerError:
				if err.NoBlockFound() {
					select {
					case <-time.After(1 * time.Minute):
						continue
					case <-ctx.Done():
						s.err = err
						return
					}
				}
			}

			if err != nil {
				s.err = err
				return
			}

			select {
			case ch <- blk:
			case <-ctx.Done():
				s.err = ctx.Err()
				return
			}

			s.head++
		}
	}()

	return ch
}
