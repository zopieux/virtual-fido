package main

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/fxamacker/cbor/v2"
)

var AAGUID = [16]byte{117, 108, 90, 245, 236, 166, 1, 163, 47, 198, 211, 12, 226, 242, 1, 197}

type CTAPCommand uint8

const (
	CTAP_COMMAND_MAKE_CREDENTIAL    CTAPCommand = 0x01
	CTAP_COMMAND_GET_ASSERTION      CTAPCommand = 0x02
	CTAP_COMMAND_GET_INFO           CTAPCommand = 0x04
	CTAP_COMMAND_CLIENT_PIN         CTAPCommand = 0x06
	CTAP_COMMAND_RESET              CTAPCommand = 0x07
	CTAP_COMMAND_GET_NEXT_ASSERTION CTAPCommand = 0x08
)

var ctapCommandDescriptions = map[CTAPCommand]string{
	CTAP_COMMAND_MAKE_CREDENTIAL:    "CTAP_COMMAND_MAKE_CREDENTIAL",
	CTAP_COMMAND_GET_ASSERTION:      "CTAP_COMMAND_GET_ASSERTION",
	CTAP_COMMAND_GET_INFO:           "CTAP_COMMAND_GET_INFO",
	CTAP_COMMAND_CLIENT_PIN:         "CTAP_COMMAND_CLIENT_PIN",
	CTAP_COMMAND_RESET:              "CTAP_COMMAND_RESET",
	CTAP_COMMAND_GET_NEXT_ASSERTION: "CTAP_COMMAND_GET_NEXT_ASSERTION",
}

type CTAPStatusCode uint8

const (
	CTAP1_ERR_SUCCESS           CTAPStatusCode = 0x00
	CTAP1_ERR_INVALID_COMMAND   CTAPStatusCode = 0x01
	CTAP1_ERR_INVALID_PARAMETER CTAPStatusCode = 0x02
	CTAP1_ERR_INVALID_LENGTH    CTAPStatusCode = 0x03
	CTAP1_ERR_INVALID_SEQ       CTAPStatusCode = 0x04
	CTAP1_ERR_TIMEOUT           CTAPStatusCode = 0x05
	CTAP1_ERR_CHANNEL_BUSY      CTAPStatusCode = 0x06

	CTAP2_ERR_UNSUPPORTED_ALGORITHM CTAPStatusCode = 0x26
)

type COSEAlgorithmID int32

const (
	COSE_ALGORITHM_ID_ES256 COSEAlgorithmID = -7
)

type PublicKeyCredentialRpEntity struct {
	Id   string `cbor:"id"`
	Name string `cbor:"name"`
}

func (rp PublicKeyCredentialRpEntity) String() string {
	return fmt.Sprintf("RpEntity{ ID: %s, Name: %s }",
		rp.Id, rp.Name)
}

type PublicKeyCrendentialUserEntity struct {
	Id          []byte `cbor:"id"`
	DisplayName string `cbor:"displayName"`
	Name        string `cbor:"name"`
}

func (user PublicKeyCrendentialUserEntity) String() string {
	return fmt.Sprintf("User{ ID: %s, DisplayName: %s, Name: %s }",
		hex.EncodeToString(user.Id),
		user.DisplayName,
		user.Name)
}

type PublicKeyCredentialDescriptor struct {
	Type       string   `cbor:"type"`
	Id         []byte   `cbor:"id"`
	Transports []string `cbor:"transports,omitempty"`
}

type PublicKeyCredentialParams struct {
	Type      string          `cbor:"type"`
	Algorithm COSEAlgorithmID `cbor:"alg"`
}

type CTAPCOSEPublicKey struct {
	Kty int8   `cbor:"1,keyasint"`
	Alg int8   `cbor:"3,keyasint"`
	Crv int8   `cbor:"-1,keyasint"`
	X   []byte `cbor:"-2,keyasint"`
	Y   []byte `cbor:"-3,keyasint"`
}

func ctapEncodeKeyAsCOSE(publicKey *ecdsa.PublicKey) []byte {
	key := CTAPCOSEPublicKey{
		Kty: 2,
		Alg: int8(COSE_ALGORITHM_ID_ES256),
		Crv: 1,
		X:   publicKey.X.Bytes(),
		Y:   publicKey.Y.Bytes(),
	}
	keyBytes, err := cbor.Marshal(key)
	checkErr(err, "Could not encode public key in COSE CBOR")
	return keyBytes
}

const (
	CTAP_AUTH_DATA_FLAG_USER_PRESENT            uint8 = 0b00000001
	CTAP_AUTH_DATA_FLAG_USER_VERIFIED           uint8 = 0b00000100
	CTAP_AUTH_DATA_FLAG_ATTESTED_DATA_INCLUDED  uint8 = 0b01000000
	CTAP_AUTH_DATA_FLAG_EXTENSION_DATA_INCLUDED uint8 = 0b10000000
)

type CTAPServer struct {
	client *Client
}

func NewCTAPServer(client *Client) *CTAPServer {
	return &CTAPServer{client: client}
}

func (server *CTAPServer) handleMessage(data []byte) []byte {
	command := CTAPCommand(data[0])
	fmt.Printf("CTAP COMMAND: %s\n\n", ctapCommandDescriptions[command])
	switch command {
	case CTAP_COMMAND_MAKE_CREDENTIAL:
		return server.handleMakeCredential(data[1:])
	case CTAP_COMMAND_GET_INFO:
		return server.handleGetInfo(data[1:])
	default:
		panic(fmt.Sprintf("Invalid CTAP Command: %d", command))
	}
}

type CTAPMakeCredentialArgsOptions struct {
	ResidentKey      bool `cbor:"rk"`
	UserVerification bool `cbor:"uv"`
}

type CTAPMakeCredentialArgs struct {
	ClientDataHash   []byte                          `cbor:"1,keyasint,omitempty"`
	Rp               PublicKeyCredentialRpEntity     `cbor:"2,keyasint,omitempty"`
	User             PublicKeyCrendentialUserEntity  `cbor:"3,keyasint,omitempty"`
	PubKeyCredParams []PublicKeyCredentialParams     `cbor:"4,keyasint,omitempty"`
	ExcludeList      []PublicKeyCredentialDescriptor `cbor:"5,keyasint,omitempty"`
	Options          CTAPMakeCredentialArgsOptions   `cbor:"7,keyasint,omitempty"`
}

func (args CTAPMakeCredentialArgs) String() string {
	return fmt.Sprintf("CTAPMakeCredentialArgs{ ClientDataHash: 0x%s, Relying Party: %s, User: %s, PublicKeyCredentialParams: %#v, ExcludeList: %#v, Options: %#v }",
		hex.EncodeToString(args.ClientDataHash),
		args.Rp,
		args.User,
		args.PubKeyCredParams,
		args.ExcludeList,
		args.Options)
}

type CTAPMakeCredentialReponse struct {
	FormatIdentifer      string                      `cbor:"1,keyasint"`
	AuthData             []byte                      `cbor:"2,keyasint"`
	AttestationStatement map[interface{}]interface{} `cbor:"3,keyasint"`
}

func (server *CTAPServer) handleMakeCredential(data []byte) []byte {
	var args CTAPMakeCredentialArgs
	err := cbor.Unmarshal(data, &args)
	checkErr(err, fmt.Sprintf("Could not decode CBOR for MAKE_CREDENTIAL: %s %v", err, data))
	fmt.Printf("MAKE CREDENTIAL: %s\n\n", args)

	supported := false
	for _, param := range args.PubKeyCredParams {
		if param.Algorithm == COSE_ALGORITHM_ID_ES256 && param.Type == "public-key" {
			supported = true
		}
	}
	if !supported {
		return []byte{byte(CTAP2_ERR_UNSUPPORTED_ALGORITHM)}
	}

	// TODO: Verify user presence and user identity (e.g. PIN, password)

	credentialSource := server.client.newCredentialSource(args.Rp.Id, args.User.Id)
	encodedCredentialPublicKey := ctapEncodeKeyAsCOSE(&credentialSource.PrivateKey.PublicKey)
	attestedCredentialData := flatten([][]byte{AAGUID[:], toBE(uint16(len(credentialSource.ID))), credentialSource.ID, encodedCredentialPublicKey})
	rpIdHash := sha256.Sum256([]byte(args.Rp.Id))
	flags := CTAP_AUTH_DATA_FLAG_USER_PRESENT | CTAP_AUTH_DATA_FLAG_USER_VERIFIED | CTAP_AUTH_DATA_FLAG_ATTESTED_DATA_INCLUDED
	authenticatorData := flatten([][]byte{rpIdHash[:], {flags}, toBE(credentialSource.SignatureCounter), attestedCredentialData})

	response := CTAPMakeCredentialReponse{
		AuthData:             authenticatorData,
		FormatIdentifer:      "none",
		AttestationStatement: make(map[interface{}]interface{}),
	}
	responseBytes, err := cbor.Marshal(response)
	checkErr(err, "Could not encode MakeAssertion response in CBOR")
	b := append([]byte{byte(CTAP1_ERR_SUCCESS)}, responseBytes...)
	fmt.Printf("MAKE_CRED: %v\n\n", b)
	return b
}

type CTAPGetInfoOptions struct {
	IsPlatform          bool `cbor:"plat,omitempty"`
	CanResidentKey      bool `cbor:"rk,omitempty"`
	CanClientPin        bool `cbor:"clientPin,omitempty"`
	CanUserPresence     bool `cbor:"up,omitempty"`
	CanUserVerification bool `cbor:"uv,omitempty"`
}

type CTAPGetInfoResponse struct {
	Versions       []string           `cbor:"1,keyasint,omitempty"`
	Extensions     []string           `cbor:"2,keyasint,omitempty"`
	AAGUID         [16]byte           `cbor:"3,keyasint,omitempty"`
	Options        CTAPGetInfoOptions `cbor:"4,keyasint,omitempty"`
	MaxMessageSize uint32             `cbor:"5,keyasint,omitempty"`
	PinProtocols   []uint32           `cbor:"6,keyasint,omitempty"`
}

func (server *CTAPServer) handleGetInfo(data []byte) []byte {
	response := CTAPGetInfoResponse{
		Versions: []string{"FIDO_2_0", "U2F_V2"},
		AAGUID:   AAGUID,
	}
	responseBytes, err := cbor.Marshal(response)
	checkErr(err, "Could not encode GET_INFO in CBOR")
	fmt.Printf("CTAP GET_INFO RESPONSE: %v\n\n", responseBytes)
	return append([]byte{byte(CTAP1_ERR_SUCCESS)}, responseBytes...)
}
