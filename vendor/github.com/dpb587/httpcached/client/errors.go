package client

type ServiceError struct {
	Err error
}

func (e ServiceError) Error() string {
	return "proxyrpc service error"
}

type BadGatewayError struct {
	s string
}

func (e BadGatewayError) Error() string {
	return e.s
}
