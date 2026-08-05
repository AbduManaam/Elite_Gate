package aws

import (
	"context"
	"errors"
	"fmt"
	"strings"

	sdkaws "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	albTypes "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
)

// gatewayALBAPI contains the low-level ELBv2 operations required by
// dedicated gateway provisioning.
type gatewayALBAPI interface {
	CreateTargetGroup(
		ctx context.Context,
		params *elasticloadbalancingv2.CreateTargetGroupInput,
		optFns ...func(*elasticloadbalancingv2.Options),
	) (*elasticloadbalancingv2.CreateTargetGroupOutput, error)

	RegisterTargets(
		ctx context.Context,
		params *elasticloadbalancingv2.RegisterTargetsInput,
		optFns ...func(*elasticloadbalancingv2.Options),
	) (*elasticloadbalancingv2.RegisterTargetsOutput, error)

	DescribeTargetHealth(
		ctx context.Context,
		params *elasticloadbalancingv2.DescribeTargetHealthInput,
		optFns ...func(*elasticloadbalancingv2.Options),
	) (*elasticloadbalancingv2.DescribeTargetHealthOutput, error)

	CreateRule(
		ctx context.Context,
		params *elasticloadbalancingv2.CreateRuleInput,
		optFns ...func(*elasticloadbalancingv2.Options),
	) (*elasticloadbalancingv2.CreateRuleOutput, error)

	DeleteRule(
		ctx context.Context,
		params *elasticloadbalancingv2.DeleteRuleInput,
		optFns ...func(*elasticloadbalancingv2.Options),
	) (*elasticloadbalancingv2.DeleteRuleOutput, error)

	DeregisterTargets(
		ctx context.Context,
		params *elasticloadbalancingv2.DeregisterTargetsInput,
		optFns ...func(*elasticloadbalancingv2.Options),
	) (*elasticloadbalancingv2.DeregisterTargetsOutput, error)

	DeleteTargetGroup(
		ctx context.Context,
		params *elasticloadbalancingv2.DeleteTargetGroupInput,
		optFns ...func(*elasticloadbalancingv2.Options),
	) (*elasticloadbalancingv2.DeleteTargetGroupOutput, error)

	DescribeTargetGroups(
		ctx context.Context,
		params *elasticloadbalancingv2.DescribeTargetGroupsInput,
		optFns ...func(*elasticloadbalancingv2.Options),
	) (*elasticloadbalancingv2.DescribeTargetGroupsOutput, error)

	DescribeRules(
		ctx context.Context,
		params *elasticloadbalancingv2.DescribeRulesInput,
		optFns ...func(*elasticloadbalancingv2.Options),
	) (*elasticloadbalancingv2.DescribeRulesOutput, error)
}

func (c *Client) gatewayALBClient() (gatewayALBAPI, error) {
	if c.cfg == nil || !c.cfg.AutomationEnabled {
		return nil, errors.New("AWS automation is disabled")
	}
	if c.albClient == nil {
		return nil, errors.New("ALB client is not initialized")
	}

	client, ok := c.albClient.(gatewayALBAPI)
	if !ok {
		return nil, errors.New("ALB client does not support dedicated gateway operations")
	}

	return client, nil
}

// CreateGatewayTargetGroup creates an HTTP target group for one dedicated gateway.
func (c *Client) CreateGatewayTargetGroup(
	ctx context.Context,
	name string,
	vpcID string,
) (string, error) {
	client, err := c.gatewayALBClient()
	if err != nil {
		return "", err
	}

	name = strings.TrimSpace(name)
	vpcID = strings.TrimSpace(vpcID)
	if name == "" || vpcID == "" {
		return "", errors.New("target group name and VPC ID are required")
	}

	existing, lookupErr := client.DescribeTargetGroups(
		ctx,
		&elasticloadbalancingv2.DescribeTargetGroupsInput{
			Names: []string{name},
		},
	)
	if lookupErr == nil && len(existing.TargetGroups) > 0 {
		arn := sdkaws.ToString(existing.TargetGroups[0].TargetGroupArn)
		if arn != "" {
			return arn, nil
		}
	}
	if lookupErr != nil && !isNotFoundError(lookupErr) {
		return "", fmt.Errorf("find existing gateway target group %s: %w", name, lookupErr)
	}

	output, err := client.CreateTargetGroup(
		ctx,
		&elasticloadbalancingv2.CreateTargetGroupInput{
			Name:                       sdkaws.String(name),
			Protocol:                   albTypes.ProtocolEnumHttp,
			Port:                       sdkaws.Int32(8080),
			VpcId:                      sdkaws.String(vpcID),
			TargetType:                 albTypes.TargetTypeEnumInstance,
			HealthCheckEnabled:         sdkaws.Bool(true),
			HealthCheckProtocol:        albTypes.ProtocolEnumHttp,
			HealthCheckPath:            sdkaws.String("/health"),
			HealthCheckPort:            sdkaws.String("traffic-port"),
			HealthyThresholdCount:      sdkaws.Int32(2),
			UnhealthyThresholdCount:    sdkaws.Int32(2),
			HealthCheckIntervalSeconds: sdkaws.Int32(15),
			HealthCheckTimeoutSeconds:  sdkaws.Int32(5),
			Matcher: &albTypes.Matcher{
				HttpCode: sdkaws.String("200"),
			},
		},
	)
	if err != nil {
		return "", fmt.Errorf("create gateway target group %s: %w", name, err)
	}

	if len(output.TargetGroups) == 0 ||
		output.TargetGroups[0].TargetGroupArn == nil {
		return "", errors.New("AWS returned no target group ARN")
	}

	return sdkaws.ToString(output.TargetGroups[0].TargetGroupArn), nil
}

// RegisterGatewayTarget registers the EC2 instance using the Docker host port.
func (c *Client) RegisterGatewayTarget(
	ctx context.Context,
	targetGroupARN string,
	instanceID string,
	port int32,
) error {
	client, err := c.gatewayALBClient()
	if err != nil {
		return err
	}

	_, err = client.RegisterTargets(
		ctx,
		&elasticloadbalancingv2.RegisterTargetsInput{
			TargetGroupArn: sdkaws.String(targetGroupARN),
			Targets: []albTypes.TargetDescription{
				{
					Id:   sdkaws.String(instanceID),
					Port: sdkaws.Int32(port),
				},
			},
		},
	)
	if err != nil {
		return fmt.Errorf("register gateway target: %w", err)
	}

	return nil
}

// GetGatewayTargetHealth returns states such as initial, healthy or unhealthy.
func (c *Client) GetGatewayTargetHealth(
	ctx context.Context,
	targetGroupARN string,
	instanceID string,
	port int32,
) (string, error) {
	client, err := c.gatewayALBClient()
	if err != nil {
		return "", err
	}

	output, err := client.DescribeTargetHealth(
		ctx,
		&elasticloadbalancingv2.DescribeTargetHealthInput{
			TargetGroupArn: sdkaws.String(targetGroupARN),
			Targets: []albTypes.TargetDescription{
				{
					Id:   sdkaws.String(instanceID),
					Port: sdkaws.Int32(port),
				},
			},
		},
	)
	if err != nil {
		return "", fmt.Errorf("describe gateway target health: %w", err)
	}

	if len(output.TargetHealthDescriptions) == 0 ||
		output.TargetHealthDescriptions[0].TargetHealth == nil {
		return "unknown", nil
	}

	return string(output.TargetHealthDescriptions[0].TargetHealth.State), nil
}

// CreateGatewayHostRule routes one gateway hostname to its target group.
func (c *Client) CreateGatewayHostRule(
	ctx context.Context,
	listenerARN string,
	hostname string,
	targetGroupARN string,
	priority int32,
) (string, error) {
	client, err := c.gatewayALBClient()
	if err != nil {
		return "", err
	}

	existing, err := client.DescribeRules(
		ctx,
		&elasticloadbalancingv2.DescribeRulesInput{
			ListenerArn: sdkaws.String(listenerARN),
		},
	)
	if err != nil {
		return "", fmt.Errorf("describe existing listener rules: %w", err)
	}

	if ruleARN, conflict := findGatewayHostRule(
		existing.Rules,
		hostname,
		targetGroupARN,
	); ruleARN != "" {
		return ruleARN, nil
	} else if conflict {
		return "", fmt.Errorf(
			"hostname %s is already routed to another target group",
			hostname,
		)
	}

	output, err := client.CreateRule(
		ctx,
		&elasticloadbalancingv2.CreateRuleInput{
			ListenerArn: sdkaws.String(listenerARN),
			Priority:    sdkaws.Int32(priority),
			Conditions: []albTypes.RuleCondition{
				{
					Field: sdkaws.String("host-header"),
					HostHeaderConfig: &albTypes.HostHeaderConditionConfig{
						Values: []string{hostname},
					},
				},
			},
			Actions: []albTypes.Action{
				{
					Type:           albTypes.ActionTypeEnumForward,
					TargetGroupArn: sdkaws.String(targetGroupARN),
				},
			},
		},
	)
	if err != nil {
		return "", fmt.Errorf("create gateway listener rule: %w", err)
	}

	if len(output.Rules) == 0 || output.Rules[0].RuleArn == nil {
		return "", errors.New("AWS returned no listener rule ARN")
	}

	return sdkaws.ToString(output.Rules[0].RuleArn), nil
}

func findGatewayHostRule(
	rules []albTypes.Rule,
	hostname string,
	targetGroupARN string,
) (ruleARN string, conflict bool) {
	expectedHost := strings.TrimSuffix(
		strings.ToLower(strings.TrimSpace(hostname)),
		".",
	)

	for _, rule := range rules {
		hostMatches := false

		for _, condition := range rule.Conditions {
			if condition.HostHeaderConfig == nil {
				continue
			}

			for _, value := range condition.HostHeaderConfig.Values {
				currentHost := strings.TrimSuffix(
					strings.ToLower(strings.TrimSpace(value)),
					".",
				)

				if currentHost == expectedHost {
					hostMatches = true
					break
				}
			}
		}

		if !hostMatches {
			continue
		}

		for _, action := range rule.Actions {
			if action.Type == albTypes.ActionTypeEnumForward &&
				sdkaws.ToString(action.TargetGroupArn) == targetGroupARN {
				return sdkaws.ToString(rule.RuleArn), false
			}
		}

		return "", true
	}

	return "", false
}

// DeleteGatewayHostRule removes the public hostname routing rule.
func (c *Client) DeleteGatewayHostRule(
	ctx context.Context,
	ruleARN string,
) error {
	client, err := c.gatewayALBClient()
	if err != nil {
		return err
	}

	_, err = client.DeleteRule(
		ctx,
		&elasticloadbalancingv2.DeleteRuleInput{
			RuleArn: sdkaws.String(ruleARN),
		},
	)
	if err != nil && !isNotFoundError(err) {
		return fmt.Errorf("delete gateway listener rule: %w", err)
	}

	return nil
}

// DeregisterGatewayTarget removes the EC2 port from the target group.
func (c *Client) DeregisterGatewayTarget(
	ctx context.Context,
	targetGroupARN string,
	instanceID string,
	port int32,
) error {
	client, err := c.gatewayALBClient()
	if err != nil {
		return err
	}

	_, err = client.DeregisterTargets(
		ctx,
		&elasticloadbalancingv2.DeregisterTargetsInput{
			TargetGroupArn: sdkaws.String(targetGroupARN),
			Targets: []albTypes.TargetDescription{
				{
					Id:   sdkaws.String(instanceID),
					Port: sdkaws.Int32(port),
				},
			},
		},
	)
	if err != nil && !isNotFoundError(err) {
		return fmt.Errorf("deregister gateway target: %w", err)
	}

	return nil
}

// DeleteGatewayTargetGroup removes the dedicated gateway target group.
func (c *Client) DeleteGatewayTargetGroup(
	ctx context.Context,
	targetGroupARN string,
) error {
	client, err := c.gatewayALBClient()
	if err != nil {
		return err
	}

	_, err = client.DeleteTargetGroup(
		ctx,
		&elasticloadbalancingv2.DeleteTargetGroupInput{
			TargetGroupArn: sdkaws.String(targetGroupARN),
		},
	)
	if err != nil && !isNotFoundError(err) {
		return fmt.Errorf("delete gateway target group: %w", err)
	}

	return nil
}
