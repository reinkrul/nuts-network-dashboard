package main

/* The types in this file map a subset of the diagostics API response of the Nuts node */

type DiagnosticsResponse struct {
	Network NetworkInfo `json:"network"`
	VDR     VDRInfo     `json:"vdr"`
	VCR     VCRInfo     `json:"vcr"`
}

type NetworkInfo struct {
	NetworkConnections NetworkConnectionsInfo `json:"connections"`
	State              NetworkStateInfo       `json:"state"`
}

type NetworkConnectionsInfo struct {
	PeerCount int `json:"connected_peers_count"`
}

type NetworkStateInfo struct {
	TransactionCount int `json:"transaction_count"`
}

type VDRInfo struct {
	DocumentCount           int `json:"did_documents_count"`
	ConflictedDocumentCount int `json:"conflicted_did_documents_count"`
}

type VCRInfo struct {
	VCCount int `json:"credential_count"`
}
