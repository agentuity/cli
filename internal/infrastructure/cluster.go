package infrastructure

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"log"
	"time"

	"github.com/agentuity/go-common/logger"
	cstr "github.com/agentuity/go-common/string"
)

type ClusterSetup interface {
	Setup(ctx context.Context, logger logger.Logger, cluster *Cluster, format string) error
}

var setups = make(map[string]ClusterSetup)

func register(provider string, setup ClusterSetup) {
	if _, ok := setups[provider]; ok {
		log.Fatalf("provider %s already registered", provider)
	}
	setups[provider] = setup
}

func Setup(ctx context.Context, logger logger.Logger, cluster *Cluster, format string) error {
	if setup, ok := setups[cluster.Provider]; ok {
		return setup.Setup(ctx, logger, cluster, format)
	}
	return fmt.Errorf("provider %s not registered", cluster.Provider)
}

func generateNodeName(prefix string) string {
	return fmt.Sprintf("%s-%s", prefix, cstr.NewHash(time.Now())[:6])
}

func generateKey() (string, string, error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", err
	}
	pkeyDER, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return "", "", err
	}
	pkeyPem := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: pkeyDER,
	})
	pubDer, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		return "", "", err
	}
	pubPem := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubDer,
	})
	return base64.StdEncoding.EncodeToString(pubPem), base64.StdEncoding.EncodeToString(pkeyPem), nil
}
