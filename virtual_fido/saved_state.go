package virtual_fido

import (
	"crypto/rand"
	"encoding/json"
	"fmt"

	"golang.org/x/crypto/scrypt"
)

type SavedCredentialSource struct {
	Type             string                         `json:"type"`
	ID               []byte                         `json:"id"`
	PrivateKey       []byte                         `json:"private_key"`
	RelyingParty     PublicKeyCredentialRpEntity    `json:"relying_party"`
	User             PublicKeyCrendentialUserEntity `json:"user"`
	SignatureCounter int32                          `json:"signature_counter"`
}

type FIDODeviceConfig struct {
	EncryptionKey          []byte                  `json:"encryption_key"`
	AttestationCertificate []byte                  `json:"attestation_certificate"`
	AttestationPrivateKey  []byte                  `json:"attestation_private_key"`
	AuthenticationCounter  uint32                  `json:"authentication_counter"`
	PINHash                []byte                  `json:"pin_hash,omitempty"`
	Sources                []SavedCredentialSource `json:"sources"`
}

type PassphraseEncryptedBlob struct {
	Salt          []byte `json:"salt"`
	EncryptionKey []byte `json:"encryption_key"`
	KeyNonce      []byte `json:"key_nonce"`
	EncryptedData []byte `json:"encrypted_data"`
	DataNonce     []byte `json:"data_nonce"`
}

func encryptPassphraseBlob(passphrase string, data []byte) (*PassphraseEncryptedBlob, error) {
	salt := read(rand.Reader, 16)
	keyEncryptionKey, err := scrypt.Key([]byte(passphrase), salt, 32768, 8, 1, 32)
	if err != nil {
		return nil, fmt.Errorf("Could not create key encryption key: %w", err)
	}
	encryptionKey := read(rand.Reader, 32)
	encryptedKey, keyNonce, err := encrypt(keyEncryptionKey, encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("Could not encrypt key: %w", err)
	}
	encryptedData, dataNonce, err := encrypt(encryptionKey, data)
	if err != nil {
		return nil, fmt.Errorf("Could not encrypt data: %w", err)
	}
	return &PassphraseEncryptedBlob{
		Salt:          salt,
		EncryptionKey: encryptedKey,
		KeyNonce:      keyNonce,
		EncryptedData: encryptedData,
		DataNonce:     dataNonce,
	}, nil
}

func decryptPassphraseBlob(passphrase string, blob PassphraseEncryptedBlob) ([]byte, error) {
	keyEncryptionKey, err := scrypt.Key([]byte(passphrase), blob.Salt, 32768, 8, 1, 32)
	checkErr(err, "Could not create key encryption key")
	encryptionKey, err := decrypt(keyEncryptionKey, blob.EncryptionKey, blob.KeyNonce)
	if err != nil {
		return nil, fmt.Errorf("Could not decrypt encryption key: %w", err)
	}
	data, err := decrypt(encryptionKey, blob.EncryptedData, blob.DataNonce)
	if err != nil {
		return nil, fmt.Errorf("Could not decrypt data: %w", err)
	}
	return data, nil
}

func EncryptWithPassphrase(savedState FIDODeviceConfig, passphrase string) ([]byte, error) {
	stateBytes, err := json.Marshal(savedState)
	if err != nil {
		return nil, fmt.Errorf("Could not encode JSON: %w", err)
	}
	blob, err := encryptPassphraseBlob(passphrase, stateBytes)
	if err != nil {
		return nil, fmt.Errorf("Could not encrypt data: %w", err)
	}
	output, err := json.Marshal(blob)
	if err != nil {
		return nil, fmt.Errorf("Could not encode JSON: %w", err)
	}
	return output, nil
}

func DecryptWithPassphrase(data []byte, passphrase string) (*FIDODeviceConfig, error) {
	blob := PassphraseEncryptedBlob{}
	err := json.Unmarshal(data, &blob)
	if err != nil {
		return nil, fmt.Errorf("Could not decode JSON: %w", err)
	}
	stateBytes, err := decryptPassphraseBlob(passphrase, blob)
	if err != nil {
		return nil, fmt.Errorf("Could not decrypt data: %w", err)
	}
	state := FIDODeviceConfig{}
	err = json.Unmarshal(stateBytes, &state)
	if err != nil {
		return nil, fmt.Errorf("Could not decode JSON: %w", err)
	}
	return &state, nil
}
