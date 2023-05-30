package keyvault

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"github.com/hashicorp/vault-client-go"
	"github.com/hashicorp/vault-client-go/schema"
	"github.com/notaryproject/notation-hashicorp-vault/internal/crypto"
	"os"
	"strings"
	"time"
)

type VaultClientWrapper struct {
	vaultClient *vault.Client

	keyID string
}

func NewVaultClientFromKeyID(id string) (*VaultClientWrapper, error) {
	// read addr and token from environment variables
	vaultAddr := os.Getenv("VAULT_ADDR")
	if len(vaultAddr) < 1 {
		return nil, errors.New("failed to load vault address")
	}

	vaultToken := os.Getenv("VAULT_TOKEN")
	if len(vaultToken) < 1 {
		return nil, errors.New("Error loading vault token")
	}

	// prepare a client with the given base address
	client, err := vault.New(
		vault.WithAddress(vaultAddr),
		vault.WithRequestTimeout(30*time.Second),
	)
	if err != nil {
		return nil, err
	}

	// authenticate with a root token (insecure)
	if err := client.SetToken(vaultToken); err != nil {
		return nil, err
	}

	return &VaultClientWrapper{
		vaultClient: client,
		keyID:       id,
	}, nil
}

func (vw *VaultClientWrapper) GetCertificateChain(ctx context.Context) ([]*x509.Certificate, error) {
	// read a certChain
	secret, err := vw.vaultClient.Secrets.KvV2Read(ctx, vw.keyID)
	if err != nil {
		return nil, err
	}
	certString, ok := secret.Data.Data["certificate"].(string)
	if !ok {
		return nil, errors.New("failed to parse certificate from KV secrets engine")
	}
	certBytes := []byte(certString)
	return crypto.ParseCertificates(certBytes)
}

func (vw *VaultClientWrapper) SignWithTransit(ctx context.Context, encodedData string, signAlgorithm string) ([]byte, error) {
	// sign with transit SE
	resp, err := vw.vaultClient.Secrets.TransitSign(ctx, vw.keyID, schema.TransitSignRequest{
		Input:               encodedData,
		MarshalingAlgorithm: "asn1",
		Prehashed:           true,
		SaltLength:          "hash",
		SignatureAlgorithm:  signAlgorithm,
	})
	if err != nil {
		return nil, err
	}

	signature, ok := resp.Data["signature"].(string)
	if !ok {
		return nil, errors.New("failed to parse signature from TransitSign response")
	}
	items := strings.Split(signature, ":")
	sigBytes, err := base64.StdEncoding.DecodeString(items[2])
	if err != nil {
		return nil, err
	}
	return sigBytes, nil
}
