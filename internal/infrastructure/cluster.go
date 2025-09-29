package infrastructure

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"log"
	"time"

	ccrypto "github.com/agentuity/go-common/crypto"
	"github.com/agentuity/go-common/logger"
	cstr "github.com/agentuity/go-common/string"
)

type ClusterSetup interface {
	Setup(ctx context.Context, logger logger.Logger, cluster *Cluster, format string) error
	CreateMachine(ctx context.Context, logger logger.Logger, region string, token string, clusterID string) error
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
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	// pubKey, err := privateKeyToPEM(privateKey)
	// if err != nil {
	// 	return "", "", err
	// }
	pkcs8BytesPK, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return "", "", err
	}
	pkcs8BytesPub, err := ccrypto.ExtractEd25519PublicKeyAsPEM(privateKey)
	if err != nil {
		return "", "", err
	}
	return string(pkcs8BytesPub), base64.StdEncoding.EncodeToString(pkcs8BytesPK), nil
}

// func privateKeyToPEM(pk ed25519.PrivateKey) ([]byte, error) {
// 	der, err := x509.MarshalPKCS8PrivateKey(pk) // PKCS#8
// 	if err != nil {
// 		return nil, err
// 	}
// 	block := &pem.Block{
// 		Type:  "PRIVATE KEY", // RFC 8410 uses unencrypted PKCS#8 with this type
// 		Bytes: der,
// 	}
// 	return pem.EncodeToMemory(block), nil
// }
