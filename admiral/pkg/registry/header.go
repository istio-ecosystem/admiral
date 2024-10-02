package registry

import (
	"fmt"
	"strings"
)

const PrivateAuthTag = "Intuit_IAM_Authentication"

const (
	TicketKey = "intuit_token"
	UserId    = "intuit_userid"
	RealmId   = "intuit_realmid"
)

type IamInfo struct {
	Authid  string
	Ticket  string
	RealmId string
}

func ParsePrivateAuthHeader(header string) (*IamInfo, error) {

	if !strings.HasPrefix(header, PrivateAuthTag) {
		return nil, fmt.Errorf("failed to parse auth header prviate auth prefix not found")
	}

	header = header[len(PrivateAuthTag)+1:]

	parts := strings.Split(header, ",")

	res := IamInfo{}

	for _, v := range parts {

		pair := strings.Split(v, "=")

		if len(pair) != 2 {
			return nil, fmt.Errorf("malformed auth header pairs didn't have two parts")
		}

		key := strings.TrimSpace(pair[0])
		value := strings.TrimSpace(pair[1])

		switch key {
		case TicketKey:
			res.Ticket = value
		case RealmId:
			res.RealmId = value
		case UserId:
			res.Authid = value
		}
	}

	return &res, nil
}
