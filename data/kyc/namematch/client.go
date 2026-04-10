// Package namematch provides the Setu Name Match API client.
//
// Returns optimistic and pessimistic match scores for two name strings.
//
// Match type thresholds:
//   - 99–100% → COMPLETE_MATCH
//   - 85–99%  → HIGH_PARTIAL_MATCH
//   - 70–85%  → MODERATE_PARTIAL_MATCH
//   - 45–70%  → LOW_PARTIAL_MATCH
//   - 0–45%   → NO_MATCH
//
// Recommended starting threshold: 70–75%.
// Setu docs: https://docs.setu.co/data/match-apis
package namematch

import (
	"context"
	"fmt"
	"net/http"

	"github.com/iamkanishka/setu-client-go/internal/transport"
	"github.com/iamkanishka/setu-client-go/pkg/setuerrors"
)

// MatchType is the qualitative similarity classification.
type MatchType string

const (
	MatchTypeComplete        MatchType = "COMPLETE_MATCH"
	MatchTypeHighPartial     MatchType = "HIGH_PARTIAL_MATCH"
	MatchTypeModeratePartial MatchType = "MODERATE_PARTIAL_MATCH"
	MatchTypeLowPartial      MatchType = "LOW_PARTIAL_MATCH"
	MatchTypeNoMatch         MatchType = "NO_MATCH"
)

// Client is the Name Match API client.
type Client struct {
	tc      *transport.Client
	baseURL string
	headers map[string]string
}

// New creates a [*Client].
func New(tc *transport.Client, baseURL string, headers map[string]string) *Client {
	return &Client{tc: tc, baseURL: baseURL, headers: headers}
}

// MatchRequest is the input for [Client.Match].
type MatchRequest struct {
	// Name1 and Name2 max 100 chars each. Only . ' - special chars allowed.
	Name1 string `json:"name1"`
	Name2 string `json:"name2"`
}

// MatchOutput holds one scoring strategy's result.
type MatchOutput struct {
	MatchType       MatchType `json:"match_type"`
	MatchPercentage float64   `json:"match_percentage"`
}

// MatchResponse is returned by [Client.Match].
type MatchResponse struct {
	ID                     string      `json:"id"`
	Name1                  string      `json:"name1"`
	Name2                  string      `json:"name2"`
	OptimisticMatchOutput  MatchOutput `json:"optimistic_match_output"`
	PessimisticMatchOutput MatchOutput `json:"pessimistic_match_output"`
	TraceID                string      `json:"traceId"`
}

// IsMatch returns true when the optimistic percentage is >= threshold.
func (r *MatchResponse) IsMatch(threshold float64) bool {
	return r.OptimisticMatchOutput.MatchPercentage >= threshold
}

// IsStrictMatch returns true when the pessimistic percentage is >= threshold.
func (r *MatchResponse) IsStrictMatch(threshold float64) bool {
	return r.PessimisticMatchOutput.MatchPercentage >= threshold
}

// Match compares two names and returns optimistic and pessimistic match scores.
//
//	POST /api/match/v1/name
func (c *Client) Match(ctx context.Context, req *MatchRequest) (*MatchResponse, error) {
	if req == nil {
		return nil, setuerrors.NewValidationError("", "request is required")
	}
	if req.Name1 == "" {
		return nil, setuerrors.NewValidationError("name1", "name1 is required")
	}
	if req.Name2 == "" {
		return nil, setuerrors.NewValidationError("name2", "name2 is required")
	}
	if len(req.Name1) > 100 {
		return nil, setuerrors.NewValidationError("name1", "name1 must not exceed 100 characters")
	}
	if len(req.Name2) > 100 {
		return nil, setuerrors.NewValidationError("name2", "name2 must not exceed 100 characters")
	}
	httpReq, err := transport.NewJSONRequest(ctx, http.MethodPost, c.baseURL+"/api/match/v1/name", req)
	if err != nil {
		return nil, fmt.Errorf("namematch: build request: %w", err)
	}
	for k, v := range c.headers {
		httpReq.Header.Set(k, v)
	}
	var out MatchResponse
	return &out, c.tc.DoJSON(httpReq, &out)
}
