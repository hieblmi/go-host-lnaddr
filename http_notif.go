package main

type httpNotificator struct {
	URL string
}

var _ notificator = (*httpNotificator)(nil)

func NewHttpNotificator(cfg notificatorConfig) *httpNotificator {
	return &httpNotificator{URL: cfg.Params["Target"]}
}

func (h *httpNotificator) Notify(amount uint64, comment string) error {
	return nil // currently not implemented
}

func (h *httpNotificator) Target() string {
	return h.URL
}
