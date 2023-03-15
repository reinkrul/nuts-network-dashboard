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
	"sort"
	"strconv"
	"sync/atomic"
	"time"
)

//go:embed web
var resources embed.FS

var nutsNodeStatusEndpoint string
var nutsNodeInternalEndpoint string
var lastDataRetrieval atomic.Pointer[time.Time]
var cachedData atomic.Pointer[[]Fact]
var lastTXsOverTimeRetrieval atomic.Pointer[time.Time]
var cachedTXsOverTime atomic.Pointer[[]CountPerMoment]
var debug = os.Getenv("DASHBOARD_DEBUG") == "1"

const maxCacheAge = 10 * time.Second

func main() {
	// Get title and Nuts node endpoint from env
	title := os.Getenv("DASHBOARD_TITLE")
	if title == "" {
		panic("DASHBOARD_TITLE not set")
	}
	nutsNodeStatusEndpoint = os.Getenv("DASHBOARD_NODE_ADDR")
	if nutsNodeStatusEndpoint == "" {
		panic("DASHBOARD_NODE_ADDR not set")
	}
	nutsNodeInternalEndpoint = os.Getenv("DASHBOARD_NODE_INTERNAL_ADDR")
	if nutsNodeInternalEndpoint == "" {
		nutsNodeInternalEndpoint = nutsNodeStatusEndpoint
	}
	log.Println("using Nuts node status base URL:", nutsNodeStatusEndpoint)
	log.Println("using Nuts node internal base URL:", nutsNodeInternalEndpoint)

	e := echo.New()
	e.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{Skipper: func(c echo.Context) bool {
		return c.Request().URL.Path == "/status"
	}}))
	_, err := os.Stat("go.mod")
	e.GET("/", echo.WrapHandler(http.FileServer(getFileSystem(err == nil))))
	e.GET("/status", func(c echo.Context) error {
		return c.String(http.StatusOK, "UP")
	})
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
	e.GET("/txs-over-time", func(c echo.Context) error {
		counts, err := readTxsOverTime(c.Request().Context())
		if err != nil {
			log.Println(err.Error())
			return c.JSON(http.StatusInternalServerError, "unable to load txs-over-time data")
		}
		var results []graphRecord
		for _, count := range counts {
			results = append(results, graphRecord{
				X: count.Moment.UnixMilli(),
				Y: count.Count,
			})
		}
		return c.JSON(http.StatusOK, results)
	})
	e.Logger.Fatal(e.Start(":8080"))
}

func readData(ctx context.Context) ([]Fact, error) {
	// Check cache
	lastRetrievalVal := lastDataRetrieval.Load()
	if lastRetrievalVal != nil && time.Since(*lastRetrievalVal) < maxCacheAge {
		// From cache
		return *cachedData.Load(), nil
	}

	httpRequest, err := http.NewRequestWithContext(ctx, "GET", nutsNodeStatusEndpoint+"/status/diagnostics", nil)
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
	lastDataRetrieval.Store(&n)
	cachedData.Store(&result)

	return result, nil
}

type graphRecord struct {
	X interface{} `json:"x"`
	Y interface{} `json:"y"`
}

type CountPerMoment struct {
	Moment time.Time `json:"moment"`
	Count  int       `json:"count"`
}

func readTxsOverTime(ctx context.Context) ([]CountPerMoment, error) {
	// Check cache
	lastRetrievalVal := lastTXsOverTimeRetrieval.Load()
	if lastRetrievalVal != nil && time.Since(*lastRetrievalVal) < maxCacheAge {
		// From cache
		return *cachedTXsOverTime.Load(), nil
	}

	const pageSize = 1000
	// entries is a map of date (in unix seconds) to number of TXs on that date
	entries := make(map[int64]int, 365*3)
	for page := 0; ; page++ {
		start := page * pageSize
		end := start + pageSize
		requestURL := nutsNodeInternalEndpoint + "/internal/network/v1/transaction?start=" + strconv.Itoa(start) + "&end=" + strconv.Itoa(end)
		request, err := http.NewRequestWithContext(ctx, "GET", requestURL, nil)
		if err != nil {
			return nil, err
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			return nil, err
		}
		responseBytes, err := io.ReadAll(response.Body)
		if err != nil {
			return nil, err
		}

		txs, err := parseTransactions(responseBytes)
		if err != nil {
			return nil, err
		}
		if len(txs) == 0 {
			// No more TXs, reached end
			break
		}

		for _, tx := range txs {
			date := tx.SigningTime.Truncate(24 * time.Hour)
			entries[date.Unix()]++
		}
	}

	var result []CountPerMoment
	for date, count := range entries {
		result = append(result, CountPerMoment{
			Moment: time.Unix(date, 0),
			Count:  count,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Moment.Before(result[j].Moment)
	})

	// Store in cache
	n := time.Now()
	lastTXsOverTimeRetrieval.Store(&n)
	cachedTXsOverTime.Store(&result)

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
