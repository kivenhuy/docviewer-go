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

var _ domain.Source = (*ServiceClient)(nil)

type ServiceClient struct {
	baseURL string
	http    *http.Client
}

type serviceDoc struct {
	TicketID    string `json:"ticket_id"`
	Description string `json:"description"`
	ServiceDate string `json:"service_date"`
	Mileage     int    `json:"mileage"`
	Workshop    string `json:"workshop"`
}

type serviceResponse struct {
	VIN  string       `json:"vin"`
	Docs []serviceDoc `json:"docs"`
}

func NewServiceClient(baseURL string, timeout time.Duration) *ServiceClient {
	return &ServiceClient{
		baseURL: baseURL,
		http:    &http.Client{Timeout: timeout},
	}
}

func (s *ServiceClient) Name() string {
	return "service"
}
func (s *ServiceClient) Fetch(ctx context.Context, vin string) ([]domain.Document, error) {
	params := url.Values{}
	params.Set("vin", vin)
	reqURL := s.baseURL + "/service/documents?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("service request: error %w", err)
	}
	if id := reqid.From(ctx); id != "" {
		req.Header.Set(reqid.Header, id) // lets a request be followed across systems
	}
	resp, err := s.http.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("service: timeout %w", err)
		}
		return nil, fmt.Errorf("service: error %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("service: unexpected status %d", resp.StatusCode)
	}
	var serviceData serviceResponse
	if err := json.NewDecoder(resp.Body).Decode(&serviceData); err != nil {
		return nil, fmt.Errorf("service: decode: %w", err)
	}
	docs := make([]domain.Document, 0, len(serviceData.Docs))
	for _, d := range serviceData.Docs {
		date, err := time.Parse("2006-01-02", d.ServiceDate)
		if err != nil {
			return nil, fmt.Errorf("service: parse date %q: %w", d.ServiceDate, err)
		}
		docs = append(docs, domain.Document{
			ID:     d.TicketID,
			Title:  d.Description,
			Type:   "service_ticket",
			Date:   date,
			Source: s.Name(),
		})
	}
	return docs, nil
}
