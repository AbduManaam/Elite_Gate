package model

type CustomDomainSync struct {
	Hostname      string `json:"hostname"`
	Status        string `json:"status"`
	RoutingStatus string `json:"routing_status"`
}
