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

type TelegramClient interface {
	SendMessage(context.Context, int64, string) error
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
	if chatID == 0 || len([]rune(text)) == 0 || len([]rune(text)) > 4096 {
		return errors.New("invalid telegram message")
	}
	body, _ := json.Marshal(map[string]any{"chat_id": chatID, "text": text, "parse_mode": "HTML", "disable_web_page_preview": true})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL+"/bot"+url.PathEscape(t.token)+"/sendMessage", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("telegram request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("telegram status %d", resp.StatusCode)
	}
	return nil
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
