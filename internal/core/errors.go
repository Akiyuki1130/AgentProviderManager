package core

type ConfigParseError struct{ Msg string }

func (e *ConfigParseError) Error() string { return e.Msg }

type ModelFetchError struct {
	Msg       string
	ErrorCode string
}

func (e *ModelFetchError) Error() string { return e.Msg }
func NewModelFetchError(msg, code string) *ModelFetchError {
	return &ModelFetchError{Msg: msg, ErrorCode: code}
}

type KeychainCryptoError struct{ Msg string }

func (e *KeychainCryptoError) Error() string { return e.Msg }

type KeychainDataError struct{ Msg string }

func (e *KeychainDataError) Error() string { return e.Msg }
