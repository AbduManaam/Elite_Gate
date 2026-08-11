package aws_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"elitegate/internal/aws"

	awsSDK "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	albTypes "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockGatewayALBAPI implements the low-level ALBAPI and gatewayALBAPI for EnsureHostRule testing
type MockGatewayALBAPI struct {
	DescribeRulesFn func(ctx context.Context, params *elasticloadbalancingv2.DescribeRulesInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.DescribeRulesOutput, error)
	CreateRuleFn    func(ctx context.Context, params *elasticloadbalancingv2.CreateRuleInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.CreateRuleOutput, error)
}

func (m *MockGatewayALBAPI) AddListenerCertificates(ctx context.Context, params *elasticloadbalancingv2.AddListenerCertificatesInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.AddListenerCertificatesOutput, error) {
	return &elasticloadbalancingv2.AddListenerCertificatesOutput{}, nil
}

func (m *MockGatewayALBAPI) RemoveListenerCertificates(ctx context.Context, params *elasticloadbalancingv2.RemoveListenerCertificatesInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.RemoveListenerCertificatesOutput, error) {
	return &elasticloadbalancingv2.RemoveListenerCertificatesOutput{}, nil
}

func (m *MockGatewayALBAPI) CreateTargetGroup(ctx context.Context, params *elasticloadbalancingv2.CreateTargetGroupInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.CreateTargetGroupOutput, error) {
	return nil, nil
}
func (m *MockGatewayALBAPI) RegisterTargets(ctx context.Context, params *elasticloadbalancingv2.RegisterTargetsInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.RegisterTargetsOutput, error) {
	return nil, nil
}
func (m *MockGatewayALBAPI) DescribeTargetHealth(ctx context.Context, params *elasticloadbalancingv2.DescribeTargetHealthInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.DescribeTargetHealthOutput, error) {
	return nil, nil
}
func (m *MockGatewayALBAPI) CreateRule(ctx context.Context, params *elasticloadbalancingv2.CreateRuleInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.CreateRuleOutput, error) {
	if m.CreateRuleFn != nil {
		return m.CreateRuleFn(ctx, params, optFns...)
	}
	pStr := ""
	if params.Priority != nil {
		pStr = fmt.Sprintf("%d", *params.Priority)
	}
	return &elasticloadbalancingv2.CreateRuleOutput{
		Rules: []albTypes.Rule{
			{
				RuleArn:  awsSDK.String("arn:aws:elasticloadbalancing:ap-south-1:123:rule/created"),
				Priority: awsSDK.String(pStr),
			},
		},
	}, nil
}
func (m *MockGatewayALBAPI) DeleteRule(ctx context.Context, params *elasticloadbalancingv2.DeleteRuleInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.DeleteRuleOutput, error) {
	return nil, nil
}
func (m *MockGatewayALBAPI) DeregisterTargets(ctx context.Context, params *elasticloadbalancingv2.DeregisterTargetsInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.DeregisterTargetsOutput, error) {
	return nil, nil
}
func (m *MockGatewayALBAPI) DeleteTargetGroup(ctx context.Context, params *elasticloadbalancingv2.DeleteTargetGroupInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.DeleteTargetGroupOutput, error) {
	return nil, nil
}
func (m *MockGatewayALBAPI) DescribeTargetGroups(ctx context.Context, params *elasticloadbalancingv2.DescribeTargetGroupsInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.DescribeTargetGroupsOutput, error) {
	return nil, nil
}
func (m *MockGatewayALBAPI) DescribeRules(ctx context.Context, params *elasticloadbalancingv2.DescribeRulesInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.DescribeRulesOutput, error) {
	if m.DescribeRulesFn != nil {
		return m.DescribeRulesFn(ctx, params, optFns...)
	}
	return &elasticloadbalancingv2.DescribeRulesOutput{Rules: []albTypes.Rule{}}, nil
}

func TestEnsureHostRule_ExistingMatchingRule_ReusesARNAndPriority(t *testing.T) {
	ctx := context.Background()
	existingARN := "arn:aws:elasticloadbalancing:ap-south-1:123:rule/existing-1"
	tgARN := "arn:aws:elasticloadbalancing:ap-south-1:123:targetgroup/tg-1/456"

	createCalled := false
	mockALB := &MockGatewayALBAPI{
		DescribeRulesFn: func(ctx context.Context, params *elasticloadbalancingv2.DescribeRulesInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.DescribeRulesOutput, error) {
			return &elasticloadbalancingv2.DescribeRulesOutput{
				Rules: []albTypes.Rule{
					{
						RuleArn:  awsSDK.String(existingARN),
						Priority: awsSDK.String("40005"),
						Conditions: []albTypes.RuleCondition{
							{
								Field: awsSDK.String("host-header"),
								HostHeaderConfig: &albTypes.HostHeaderConditionConfig{
									Values: []string{"app.example.com"},
								},
							},
						},
						Actions: []albTypes.Action{
							{
								Type:           albTypes.ActionTypeEnumForward,
								TargetGroupArn: awsSDK.String(tgARN),
							},
						},
					},
				},
			}, nil
		},
		CreateRuleFn: func(ctx context.Context, params *elasticloadbalancingv2.CreateRuleInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.CreateRuleOutput, error) {
			createCalled = true
			return nil, nil
		},
	}

	cfg, _ := aws.ValidateAWSConfig("ap-south-1", "arn:aws:listener", true)
	client := aws.NewClientWithAPIs(&MockSDKACM{}, mockALB, cfg)

	arn, priority, err := client.EnsureHostRule(ctx, "arn:aws:listener", "App.Example.COM.", tgARN, 40001, 50000)
	require.NoError(t, err)
	assert.Equal(t, existingARN, arn)
	assert.Equal(t, int32(40005), priority)
	assert.False(t, createCalled, "CreateRule must not be called when rule exists")
}

func TestEnsureHostRule_TargetConflict_ReturnsError(t *testing.T) {
	ctx := context.Background()
	existingTG := "arn:aws:targetgroup/tg-other"
	requestedTG := "arn:aws:targetgroup/tg-mine"

	mockALB := &MockGatewayALBAPI{
		DescribeRulesFn: func(ctx context.Context, params *elasticloadbalancingv2.DescribeRulesInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.DescribeRulesOutput, error) {
			return &elasticloadbalancingv2.DescribeRulesOutput{
				Rules: []albTypes.Rule{
					{
						RuleArn:  awsSDK.String("arn:aws:rule/conflict"),
						Priority: awsSDK.String("40001"),
						Conditions: []albTypes.RuleCondition{
							{
								Field: awsSDK.String("host-header"),
								HostHeaderConfig: &albTypes.HostHeaderConditionConfig{
									Values: []string{"app.example.com"},
								},
							},
						},
						Actions: []albTypes.Action{
							{
								Type:           albTypes.ActionTypeEnumForward,
								TargetGroupArn: awsSDK.String(existingTG),
							},
						},
					},
				},
			}, nil
		},
	}

	cfg, _ := aws.ValidateAWSConfig("ap-south-1", "arn:aws:listener", true)
	client := aws.NewClientWithAPIs(&MockSDKACM{}, mockALB, cfg)

	_, _, err := client.EnsureHostRule(ctx, "arn:aws:listener", "app.example.com", requestedTG, 40001, 50000)
	require.Error(t, err)
	assert.True(t, errors.Is(err, aws.ErrHostRuleTargetConflict))
}

func TestEnsureHostRule_SelectsFirstFreePriority(t *testing.T) {
	ctx := context.Background()
	tgARN := "arn:aws:targetgroup/tg-1"

	var chosenPriority int32
	mockALB := &MockGatewayALBAPI{
		DescribeRulesFn: func(ctx context.Context, params *elasticloadbalancingv2.DescribeRulesInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.DescribeRulesOutput, error) {
			return &elasticloadbalancingv2.DescribeRulesOutput{
				Rules: []albTypes.Rule{
					{Priority: awsSDK.String("default")},
					{Priority: awsSDK.String("40001")},
					{Priority: awsSDK.String("40002")},
				},
			}, nil
		},
		CreateRuleFn: func(ctx context.Context, params *elasticloadbalancingv2.CreateRuleInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.CreateRuleOutput, error) {
			chosenPriority = awsSDK.ToInt32(params.Priority)
			pStr := ""
			if params.Priority != nil {
				pStr = fmt.Sprintf("%d", *params.Priority)
			}
			return &elasticloadbalancingv2.CreateRuleOutput{
				Rules: []albTypes.Rule{
					{
						RuleArn:  awsSDK.String("arn:aws:rule/new"),
						Priority: awsSDK.String(pStr),
					},
				},
			}, nil
		},
	}

	cfg, _ := aws.ValidateAWSConfig("ap-south-1", "arn:aws:listener", true)
	client := aws.NewClientWithAPIs(&MockSDKACM{}, mockALB, cfg)

	arn, priority, err := client.EnsureHostRule(ctx, "arn:aws:listener", "new.example.com", tgARN, 40001, 50000)
	require.NoError(t, err)
	assert.Equal(t, "arn:aws:rule/new", arn)
	assert.Equal(t, int32(40003), priority)
	assert.Equal(t, int32(40003), chosenPriority)
}

func TestEnsureHostRule_PriorityInUseRace_RetriesAndSucceeds(t *testing.T) {
	ctx := context.Background()
	tgARN := "arn:aws:targetgroup/tg-1"

	attempt := 0
	mockALB := &MockGatewayALBAPI{
		DescribeRulesFn: func(ctx context.Context, params *elasticloadbalancingv2.DescribeRulesInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.DescribeRulesOutput, error) {
			attempt++
			if attempt == 1 {
				// Initial describe: 40001 is free
				return &elasticloadbalancingv2.DescribeRulesOutput{Rules: []albTypes.Rule{}}, nil
			}
			// Second describe after PriorityInUse: another process took 40001
			return &elasticloadbalancingv2.DescribeRulesOutput{
				Rules: []albTypes.Rule{
					{Priority: awsSDK.String("40001")},
				},
			}, nil
		},
		CreateRuleFn: func(ctx context.Context, params *elasticloadbalancingv2.CreateRuleInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.CreateRuleOutput, error) {
			if awsSDK.ToInt32(params.Priority) == 40001 {
				return nil, &albTypes.PriorityInUseException{Message: awsSDK.String("Priority 40001 in use")}
			}
			pStr := ""
			if params.Priority != nil {
				pStr = fmt.Sprintf("%d", *params.Priority)
			}
			return &elasticloadbalancingv2.CreateRuleOutput{
				Rules: []albTypes.Rule{
					{
						RuleArn:  awsSDK.String("arn:aws:rule/retry-success"),
						Priority: awsSDK.String(pStr),
					},
				},
			}, nil
		},
	}

	cfg, _ := aws.ValidateAWSConfig("ap-south-1", "arn:aws:listener", true)
	client := aws.NewClientWithAPIs(&MockSDKACM{}, mockALB, cfg)

	arn, priority, err := client.EnsureHostRule(ctx, "arn:aws:listener", "race.example.com", tgARN, 40001, 50000)
	require.NoError(t, err)
	assert.Equal(t, "arn:aws:rule/retry-success", arn)
	assert.Equal(t, int32(40002), priority)
}

func TestEnsureHostRule_RangeExhausted_ReturnsError(t *testing.T) {
	ctx := context.Background()

	mockALB := &MockGatewayALBAPI{
		DescribeRulesFn: func(ctx context.Context, params *elasticloadbalancingv2.DescribeRulesInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.DescribeRulesOutput, error) {
			return &elasticloadbalancingv2.DescribeRulesOutput{
				Rules: []albTypes.Rule{
					{Priority: awsSDK.String("40001")},
					{Priority: awsSDK.String("40002")},
				},
			}, nil
		},
	}

	cfg, _ := aws.ValidateAWSConfig("ap-south-1", "arn:aws:listener", true)
	client := aws.NewClientWithAPIs(&MockSDKACM{}, mockALB, cfg)

	_, _, err := client.EnsureHostRule(ctx, "arn:aws:listener", "full.example.com", "arn:aws:tg/1", 40001, 40002)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no available listener rule priority")
}

func TestEnsureHostRule_HostnameNormalization(t *testing.T) {
	ctx := context.Background()
	tgARN := "arn:aws:targetgroup/tg-1"

	var sentHostname string
	mockALB := &MockGatewayALBAPI{
		CreateRuleFn: func(ctx context.Context, params *elasticloadbalancingv2.CreateRuleInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.CreateRuleOutput, error) {
			require.NotEmpty(t, params.Conditions)
			require.NotNil(t, params.Conditions[0].HostHeaderConfig)
			sentHostname = params.Conditions[0].HostHeaderConfig.Values[0]
			pStr := ""
			if params.Priority != nil {
				pStr = fmt.Sprintf("%d", *params.Priority)
			}
			return &elasticloadbalancingv2.CreateRuleOutput{
				Rules: []albTypes.Rule{
					{
						RuleArn:  awsSDK.String("arn:aws:rule/norm"),
						Priority: awsSDK.String(pStr),
					},
				},
			}, nil
		},
	}

	cfg, _ := aws.ValidateAWSConfig("ap-south-1", "arn:aws:listener", true)
	client := aws.NewClientWithAPIs(&MockSDKACM{}, mockALB, cfg)

	_, _, err := client.EnsureHostRule(ctx, "arn:aws:listener", "   App.EXAMPLE.com.  ", tgARN, 40001, 50000)
	require.NoError(t, err)
	assert.Equal(t, "app.example.com", sentHostname)
}
