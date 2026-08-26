package cbr

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/encoding/charmap"
)

const (
	Provider = "cbr"
	Endpoint = "https://www.cbr.ru/scripts/XML_daily.asp"
)

type Rate struct {
	Date          time.Time
	Currency      string
	Nominal       int
	SourceValue   string
	RubPerUnit    string
	Revision      string
	ProvenanceURL string
}

type Client struct {
	HTTP    *http.Client
	BaseURL string
}

type response struct {
	Date  string    `xml:"Date,attr"`
	Name  string    `xml:"name,attr"`
	Rates []xmlRate `xml:"Valute"`
}

type xmlRate struct {
	ID      string `xml:"ID,attr"`
	Code    string `xml:"CharCode"`
	Nominal int    `xml:"Nominal"`
	Value   string `xml:"Value"`
}

func (c Client) Fetch(ctx context.Context, requested time.Time) ([]Rate, error) {
	baseURL := c.BaseURL
	if baseURL == "" {
		baseURL = Endpoint
	}
	endpoint, _ := url.Parse(baseURL)
	query := endpoint.Query()
	query.Set("date_req", requested.UTC().Format("02/01/2006"))
	endpoint.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("cbr request: %w", err)
	}
	req.Header.Set("Accept", "application/xml")
	req.Header.Set("User-Agent", "ITLaborPulse/1.0 (+https://github.com/Chuchoss/it-labor-pulse)")
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cbr fetch: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
		return nil, fmt.Errorf("cbr fetch: HTTP %d", res.StatusCode)
	}
	var payload response
	decoder := xml.NewDecoder(io.LimitReader(res.Body, 2<<20))
	decoder.CharsetReader = func(label string, input io.Reader) (io.Reader, error) {
		if strings.EqualFold(label, "windows-1251") {
			return charmap.Windows1251.NewDecoder().Reader(input), nil
		}
		return input, nil
	}
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("cbr decode: %w", err)
	}
	rateDate, err := time.Parse("02.01.2006", payload.Date)
	if err != nil {
		return nil, fmt.Errorf("cbr response date: %w", err)
	}
	result := make([]Rate, 0, len(payload.Rates))
	for _, item := range payload.Rates {
		if item.Nominal <= 0 || len(item.Code) != 3 {
			continue
		}
		value, err := parseDecimal(item.Value)
		if err != nil || value <= 0 {
			return nil, fmt.Errorf("cbr invalid rate %s", item.Code)
		}
		result = append(result, Rate{
			Date: rateDate.UTC(), Currency: strings.ToUpper(item.Code),
			Nominal: item.Nominal, SourceValue: decimal(item.Value),
			RubPerUnit: strconv.FormatFloat(value/float64(item.Nominal), 'f', 10, 64),
			Revision:   item.ID, ProvenanceURL: endpoint.String(),
		})
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("cbr response contains no rates")
	}
	return result, nil
}

func parseDecimal(value string) (float64, error) {
	return strconv.ParseFloat(decimal(value), 64)
}

func decimal(value string) string {
	return strings.ReplaceAll(strings.TrimSpace(value), ",", ".")
}
