package cbr

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFetchNormalizesNominalAndResponseDate(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "25/08/2026", r.URL.Query().Get("date_req"))
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="windows-1251"?>
<ValCurs Date="22.08.2026" name="Foreign Currency Market">
  <Valute ID="R01235"><CharCode>USD</CharCode><Nominal>1</Nominal><Value>80,0000</Value></Valute>
  <Valute ID="R01335"><CharCode>KZT</CharCode><Nominal>100</Nominal><Value>18,5000</Value></Valute>
  <Valute ID="R01060"><CharCode>AMD</CharCode><Nominal>100</Nominal><Value>20,7500</Value></Valute>
</ValCurs>`))
	}))
	defer server.Close()

	rates, err := (Client{BaseURL: server.URL}).Fetch(
		context.Background(),
		time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC),
	)
	require.NoError(t, err)
	require.Len(t, rates, 3)
	require.Equal(t, "80.0000000000", rates[0].RubPerUnit)
	require.Equal(t, "0.1850000000", rates[1].RubPerUnit)
	require.Equal(t, "2026-08-22", rates[1].Date.Format(time.DateOnly))
	require.Equal(t, "0.2075000000", rates[2].RubPerUnit)
}
