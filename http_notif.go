package main

type httpNotificator struct {
	URL string
}

func NewHttpNotificator(cfg notificatorConfig) *httpNotificator {
	return &httpNotificator{URL: cfg.Target}
}

func (h *httpNotificator) Notify(amount uint) error {
	return nil // currently not implemented
}

func (h *httpNotificator) Target() string {
	return h.URL
}
