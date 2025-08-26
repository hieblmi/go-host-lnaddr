package notifier

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"text/template"
)

type encoding string

func (e encoding) AddHeader(header http.Header) {
	header.Add("Content-Type", string(e))
}

func (e encoding) EscapeValue(value string) string {
	switch e {
	case EncodingForm:
		return template.URLQueryEscaper(value)

	case EncodingJson:
		return template.JSEscapeString(value)

	default:
		return value
	}
}

const (
	EncodingForm encoding = "application/x-www-form-urlencoded"
	EncodingJson encoding = "application/json"
)

type HttpNotifier struct {
	Cfg          Config
	URL          string
	Method       string
	Encoding     encoding
	BodyTemplate string
}

var _ Notifier = (*HttpNotifier)(nil)

func NewHttpNotifier(cfg Config) *HttpNotifier {
	return &HttpNotifier{
		Cfg:          cfg,
		URL:          cfg.Params["Target"],
		Method:       cfg.Params["Method"],
		Encoding:     encoding(cfg.Params["Encoding"]),
		BodyTemplate: cfg.Params["BodyTemplate"],
	}
}

func (h *HttpNotifier) Notify(amount uint64, comment string) error {
	bodyData := &struct {
		Amount  uint64
		Message string
	}{
		Amount:  amount,
		Message: h.Encoding.EscapeValue(comment),
	}

	urlTemplate, err := template.New("url").Parse(h.URL)
	if err != nil {
		return fmt.Errorf("error building URL template: %w", err)
	}

	bodyTemplate, err := template.New("body").Parse(h.BodyTemplate)
	if err != nil {
		return fmt.Errorf("error building body template: %w", err)
	}

	var buf bytes.Buffer
	err = urlTemplate.Execute(&buf, bodyData)
	if err != nil {
		return fmt.Errorf("error executing URL template: %w", err)
	}
	url := buf.String()

	buf.Reset()
	err = bodyTemplate.Execute(&buf, bodyData)
	if err != nil {
		return fmt.Errorf("error executing body template: %w", err)
	}

	var bodyReader io.Reader
	if h.Method == http.MethodPost {
		bodyReader = &buf
	}

	req, err := http.NewRequest(h.Method, url, bodyReader)
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	h.Encoding.AddHeader(req.Header)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("error sending request: %w", err)
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error reading response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d (%s)",
			resp.StatusCode, body)
	}

	return nil
}

func (h *HttpNotifier) Target() string {
	return h.URL
}
