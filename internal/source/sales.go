package source

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/kivenhuy/docviewer-go/internal/domain"
	"github.com/kivenhuy/docviewer-go/internal/reqid"
)

var _ domain.Source = (*SalesClient)(nil)

type SalesClient struct {
	baseURL string
	http    *http.Client
}
type salesDoc struct {
	SaleCode    string `json:"sale_code"`
	SaleNumber  string `json:"sale_number"`
	Title       string `json:"sale_title"`
	Type        string `json:"sale_type"`
	CreatedDate string `json:"created"`
}

type saleResponse struct {
	VIN  string     `json:"vin"`
	Docs []salesDoc `json:"docs"`
}

func NewSalesClient(baseURL string, timeout time.Duration) *SalesClient {
	return &SalesClient{
		baseURL: baseURL,
		http:    &http.Client{Timeout: timeout},
	}
}

func (s *SalesClient) Name() string {
	return "sales"
}

func (s *SalesClient) Fetch(ctx context.Context, vin string) ([]domain.Document, error) {
	params := url.Values{}
	params.Set("vin", vin)
	reqURL := s.baseURL + "/sales/documents?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("sales request: error %w", err)
	}
	if id := reqid.From(ctx); id != "" {
		req.Header.Set(reqid.Header, id) // lets a request be followed across systems
	}
	resp, err := s.http.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("sales: timeout %w", err)
		}
		return nil, fmt.Errorf("sales: error %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sales: unexpected status %d", resp.StatusCode)
	}
	var saleData saleResponse
	if err := json.NewDecoder(resp.Body).Decode(&saleData); err != nil {
		return nil, fmt.Errorf("sales: decode: %w", err)
	}
	docs := make([]domain.Document, 0, len(saleData.Docs))
	for _, d := range saleData.Docs {
		date, err := time.Parse("2006-01-02", d.CreatedDate)
		if err != nil {
			return nil, fmt.Errorf("sales: parse date %q: %w", d.CreatedDate, err)
		}
		docs = append(docs, domain.Document{
			ID:     d.SaleCode,
			Title:  d.Title,
			Type:   d.Type,
			Date:   date,
			Source: s.Name(),
		})
	}
	return docs, nil

}
