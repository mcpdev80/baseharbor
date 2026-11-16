package application

import "github.com/mcpdev80/baseharbor/internal/stableid"

func NewApplicationID() (string, error) {
	return stableid.NewUUIDv4("application")
}

func MustNewApplicationID() string {
	id, err := NewApplicationID()
	if err != nil {
		panic(err)
	}
	return id
}

func ValidateApplicationID(id string) error {
	return stableid.ValidateUUIDv4("application", id)
}
