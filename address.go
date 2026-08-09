package mailer

import (
	"fmt"
	mailpkg "net/mail"
	"strings"
)

type Address struct {
	Email string
	Name  string
}

func NewAddress(email string, name string) (Address, error) {
	parsed, err := parseAddress(email)
	if err != nil {
		return Address{}, err
	}

	return Address{
		Email: parsed,
		Name:  strings.TrimSpace(name),
	}, nil
}

func ParseAddressValue(raw string) (Address, error) {
	parsed, err := mailpkg.ParseAddress(strings.TrimSpace(raw))
	if err != nil {
		return Address{}, err
	}

	return Address{
		Email: parsed.Address,
		Name:  parsed.Name,
	}, nil
}

func (a Address) String() (string, error) {
	mailbox, err := a.mailAddress()
	if err != nil {
		return "", err
	}

	return mailbox.String(), nil
}

func (a Address) mailAddress() (*mailpkg.Address, error) {
	parsed, err := parseAddress(a.Email)
	if err != nil {
		return nil, err
	}

	return &mailpkg.Address{
		Address: parsed,
		Name:    strings.TrimSpace(a.Name),
	}, nil
}

func (a Address) MustString() string {
	value, err := a.String()
	if err != nil {
		panic(fmt.Sprintf("invalid address: %v", err))
	}

	return value
}
