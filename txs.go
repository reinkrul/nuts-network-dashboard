package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

func parseTransactions(data []byte) ([]transaction, error) {
	var items []string
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("unable to unmarshal transactions list into []string: %w", err)
	}

	var txs []transaction
	for _, item := range items {
		// Parse JWS, split into parts, and parse the headers
		parts := strings.Split(item, ".")
		if len(parts) != 3 {
			return nil, fmt.Errorf("invalid JWS: %s", item)
		}
		headersBytes, err := base64.RawStdEncoding.DecodeString(parts[0])
		if err != nil {
			return nil, fmt.Errorf("unable to base64 decode JWS headers: %w", err)
		}
		var headers map[string]interface{}
		if err := json.Unmarshal(headersBytes, &headers); err != nil {
			return nil, fmt.Errorf("unable to unmarshal JWS headers: %w", err)
		}
		if _, ok := headers["sigt"].(float64); !ok {
			log.Println("TX with invalid signing time, skipping")
			continue
		}
		signingTime := time.Unix(int64(headers["sigt"].(float64)), 0).UTC()
		txs = append(txs, transaction{SigningTime: signingTime})
	}
	return txs, nil
}

type transaction struct {
	SigningTime time.Time
}
