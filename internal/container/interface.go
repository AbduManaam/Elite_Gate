package container

import "context"

type ContainerManager interface {
	Provision(ctx context.Context, gatewayID string, projectID string, plan string) (endpointIP, gatewayPort, publicHost, publicPort string, err error)
	Decommission(ctx context.Context, gatewayID string) error
}
