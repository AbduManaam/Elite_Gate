package container

import "context"

type ContainerManager interface {
	Provision(ctx context.Context, gatewayID string, projectID string, plan string) (endpointIP string, gatewayPort string, err error)
	Decommission(ctx context.Context, gatewayID string) error
}
