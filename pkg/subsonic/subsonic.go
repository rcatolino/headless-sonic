package subsonic

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"headless-sonic/pkg/config"
	"net/http"
	"os"

	"github.com/supersonic-app/go-subsonic/subsonic"
)

func NewClient(cfg *config.Config) (*subsonic.Client, error) {
	pool, err := x509.SystemCertPool()
	if err != nil {
		return nil, err
	}

	if cfg.CustomCa != "" {
		caPEM, err := os.ReadFile(cfg.CustomCa)
		if err != nil {
			return nil, err
		}

		ok := pool.AppendCertsFromPEM(caPEM)
		if !ok {
			return nil, fmt.Errorf("Error decoding pem certificate from custom CA file '%s'", cfg.CustomCa)
		}
	}

	httpClient := http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: pool,
			},
		},
	}

	sc := &subsonic.Client{
		Client:       &httpClient,
		BaseUrl:      cfg.ServerUrl,
		User:         cfg.Username,
		ClientName:   "headless-sonic",
		PasswordAuth: true,
	}

	err = sc.Authenticate(cfg.Password)
	if err != nil {
		return nil, err
	}

	return sc, nil
}
