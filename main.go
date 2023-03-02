package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"sync/atomic"
	"time"
)

//go:embed web
var resources embed.FS

var nutsNodeEndpoint string
var lastRetrieval atomic.Pointer[time.Time]
var cachedData atomic.Pointer[[]Fact]
var debug = os.Getenv("DASHBOARD_DEBUG") == "1"

const maxCacheAge = 10 * time.Second

func main() {
	// Get title and Nuts node endpoint from env
	title := os.Getenv("DASHBOARD_TITLE")
	if title == "" {
		panic("DASHBOARD_TITLE not set")
	}
	nutsNodeEndpoint = os.Getenv("DASHBOARD_NODE_ADDR")
	if nutsNodeEndpoint == "" {
		panic("DASHBOARD_NODE_ADDR not set")
	}

	e := echo.New()
	e.Use(middleware.Logger())
	_, err := os.Stat("go.mod")
	e.GET("/", echo.WrapHandler(http.FileServer(getFileSystem(err == nil))))
	e.GET("/data", func(c echo.Context) error {
		facts, err := readData(c.Request().Context())
		if err != nil {
			log.Println(err.Error())
			return c.JSON(http.StatusInternalServerError, "unable to load data")
		}
		return c.JSON(http.StatusOK, GetDataResponse{
			Title: title,
			Facts: facts,
		})
	})
	e.Logger.Fatal(e.Start(":8080"))
}

func readData(ctx context.Context) ([]Fact, error) {
	// Check cache
	lastRetrievalVal := lastRetrieval.Load()
	if lastRetrievalVal != nil && time.Since(*lastRetrievalVal) < maxCacheAge {
		// From cache
		return *cachedData.Load(), nil
	}

	httpRequest, err := http.NewRequestWithContext(ctx, "GET", nutsNodeEndpoint+"/status/diagnostics", nil)
	if err != nil {
		return nil, fmt.Errorf("error creating HTTP request: %w", err)
	}
	httpRequest.Header.Add("Accept", "application/json")
	httpResponse, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		return nil, fmt.Errorf("error reading Nuts node status: %w", err)
	}
	dataBytes, err := io.ReadAll(httpResponse.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	if debug {
		log.Printf("status response: %s\n", string(dataBytes))
	}

	var diag DiagnosticsResponse
	err = json.Unmarshal(dataBytes, &diag)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling diagnostics: %w", err)
	}

	result := []Fact{
		{
			Unit:  "nodes",
			Value: diag.Network.NetworkConnections.PeerCount,
		},
		{
			Unit:  "TXs",
			Value: diag.Network.State.TransactionCount,
		},
		{
			Unit:  "DID documents",
			Value: diag.VDR.DocumentCount,
		},
		{
			Unit:  "DID document conflicts",
			Value: diag.VDR.ConflictedDocumentCount,
		},
		{
			Unit:  "Verifiable Credentials",
			Value: diag.VCR.VCCount,
		},
	}
	// Store in cache
	n := time.Now()
	lastRetrieval.Store(&n)
	cachedData.Store(&result)

	return result, nil
}

func getFileSystem(useOS bool) http.FileSystem {
	if useOS {
		log.Print("using live mode")
		return http.FS(os.DirFS("web"))
	}
	fsys, err := fs.Sub(resources, "web")
	if err != nil {
		panic(err)
	}

	return http.FS(fsys)
}

type GetDataResponse struct {
	Title string `json:"title"`
	Facts []Fact `json:"facts"`
}

type Fact struct {
	Value interface{} `json:"value"`
	Unit  string      `json:"unit"`
}
