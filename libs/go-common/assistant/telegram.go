package assistant

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type TelegramResponse struct {
	MessageID int
}

type TelegramUpdate struct {
	UpdateID int `json:"update_id"`
	Message  *struct {
		Chat struct {
			ID int64 `json:"id"`
		} `json:"chat"`
		Text string `json:"text"`
	} `json:"message"`
}

type TelegramError struct {
	StatusCode int
	RetryAfter time.Duration
}

func (e *TelegramError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("telegram status %d retry_after=%s", e.StatusCode, e.RetryAfter)
	}
	return fmt.Sprintf("telegram status %d", e.StatusCode)
}

type TelegramClient interface {
	SendMessage(context.Context, int64, string) error
}

type DeliveryTelegramClient interface {
	SendMessageResult(context.Context, int64, string) (TelegramResponse, error)
}

type Telegram struct {
	token   string
	baseURL string
	client  *http.Client
}

func NewTelegram(token string, client *http.Client) (*Telegram, error) {
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("telegram bot token is not configured")
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &Telegram{token: token, baseURL: "https://api.telegram.org", client: client}, nil
}

func (t *Telegram) SendMessage(ctx context.Context, chatID int64, text string) error {
	_, err := t.SendMessageResult(ctx, chatID, text)
	return err
}

func (t *Telegram) SendMessageResult(ctx context.Context, chatID int64, text string) (TelegramResponse, error) {
	if chatID == 0 || len([]rune(text)) == 0 || len([]rune(text)) > 4096 {
		return TelegramResponse{}, errors.New("invalid telegram message")
	}
	body, _ := json.Marshal(map[string]any{"chat_id": chatID, "text": text, "parse_mode": "HTML", "disable_web_page_preview": true})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL+"/bot"+url.PathEscape(t.token)+"/sendMessage", bytes.NewReader(body))
	if err != nil {
		return TelegramResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.client.Do(req)
	if err != nil {
		return TelegramResponse{}, fmt.Errorf("telegram request: %w", err)
	}
	defer resp.Body.Close()
	var payload struct {
		OK     bool `json:"ok"`
		Result struct {
			MessageID int `json:"message_id"`
		} `json:"result"`
		Parameters struct {
			RetryAfter int `json:"retry_after"`
		} `json:"parameters"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&payload)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		retryAfter := time.Duration(payload.Parameters.RetryAfter) * time.Second
		return TelegramResponse{}, &TelegramError{StatusCode: resp.StatusCode, RetryAfter: retryAfter}
	}
	if !payload.OK || payload.Result.MessageID == 0 {
		return TelegramResponse{}, errors.New("telegram response was not successful")
	}
	return TelegramResponse{MessageID: payload.Result.MessageID}, nil
}

func (t *Telegram) GetUpdates(ctx context.Context, offset int) ([]TelegramUpdate, error) {
	body, _ := json.Marshal(map[string]any{"offset": offset, "timeout": 20, "allowed_updates": []string{"message"}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL+"/bot"+url.PathEscape(t.token)+"/getUpdates", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("telegram updates request: %w", err)
	}
	defer resp.Body.Close()
	var payload struct {
		OK     bool             `json:"ok"`
		Result []TelegramUpdate `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || !payload.OK {
		return nil, &TelegramError{StatusCode: resp.StatusCode}
	}
	return payload.Result, nil
}

func TelegramHTML(title, salary, sourceURL string, score float64, confidence string, reasons []string) string {
	var b strings.Builder
	b.WriteString("<b>Подходящая вакансия</b>\n")
	b.WriteString("<b>")
	b.WriteString(htmlEscape(title))
	b.WriteString("</b>\n")
	if salary != "" {
		b.WriteString(htmlEscape(salary))
		b.WriteByte('\n')
	}
	b.WriteString("Совпадение: ")
	b.WriteString(strconv.FormatFloat(score*100, 'f', 0, 64))
	b.WriteString("% · уверенность: ")
	b.WriteString(htmlEscape(confidence))
	for _, reason := range reasons {
		b.WriteString("\n• ")
		b.WriteString(htmlEscape(reason))
	}
	if u, err := url.Parse(sourceURL); err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" {
		b.WriteString("\n<a href=\"")
		b.WriteString(htmlEscape(sourceURL))
		b.WriteString("\">Открыть источник</a>")
	}
	b.WriteString("\nИсточник: HeadHunter (официальная ссылка)")
	return b.String()
}

func htmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;")
	return r.Replace(s)
}
