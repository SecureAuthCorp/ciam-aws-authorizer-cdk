package authorizer

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/assertions"
	"github.com/aws/jsii-runtime-go"
)

type (
	obj = map[string]interface{}
	arr = []interface{}
)

const (
	lambdaType       = "AWS::Lambda::Function"
	queueType        = "AWS::SQS::Queue"
	stateMachineType = "AWS::StepFunctions::StateMachine"
	ruleType         = "AWS::Events::Rule"
	policyType       = "AWS::IAM::Policy"
)

func TestMain(m *testing.M) {
	code := m.Run()
	jsii.Close()
	os.Exit(code)
}

func validProps() StackProps {
	return StackProps{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		IssuerURL:    "https://acp.example.com/default/default",
		Version:      "2.22.0",
	}
}

func withEnv(props StackProps) StackProps {
	props.Env = &awscdk.Environment{Account: jsii.String("123456789012"), Region: jsii.String("us-east-1")}
	return props
}

func synth(t *testing.T, props StackProps) (Stack, assertions.Template) {
	t.Helper()
	stack, err := NewStack(awscdk.NewApp(nil), "Test", props)
	if err != nil {
		t.Fatalf("NewStack: %v", err)
	}
	return stack, assertions.Template_FromStack(awscdk.Stack_Of(stack.AuthorizerLambda), nil)
}

func logicalID(t *testing.T, template assertions.Template, resourceType string, props interface{}) string {
	t.Helper()
	resources := *template.FindResources(jsii.String(resourceType), props)
	if len(resources) != 1 {
		t.Fatalf("expected exactly one %s matching %v, got %d", resourceType, props, len(resources))
	}
	for id := range resources {
		return id
	}
	return ""
}

func constructLogicalID(c awscdk.IResource) string {
	return *awscdk.Stack_Of(c).GetLogicalId(c.Node().DefaultChild().(awscdk.CfnElement))
}

func getAtt(id, attr string) obj {
	return obj{"Fn::GetAtt": arr{id, attr}}
}

func s3BucketPerRegion(name string) obj {
	return obj{"Fn::Join": arr{"", arr{name + "-", obj{"Ref": "AWS::Region"}}}}
}

func emptyZip(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAuthorizerLambda(t *testing.T) {
	_, template := synth(t, validProps())

	template.HasResourceProperties(jsii.String(lambdaType), obj{
		"Handler":    "bootstrap",
		"Runtime":    "provided.al2023",
		"MemorySize": 128,
		"Timeout":    10,
		"Code": obj{
			"S3Bucket": s3BucketPerRegion("cloudentity-aws-api-gateway-authorizer"),
			"S3Key":    "cloudentity-aws-authorizer-v2-2.22.0.zip",
		},
		"Environment": obj{"Variables": assertions.Match_ObjectEquals(&obj{
			"ACP_CLIENT_ID":                              "client-id",
			"ACP_CLIENT_SECRET":                          "client-secret",
			"ACP_ISSUER_URL":                             "https://acp.example.com/default/default",
			"LOGGING_LEVEL":                              "info",
			"ANALYTICS_ENABLED":                          "true",
			"HTTP_CLIENT_ROOT_CA":                        "",
			"HTTP_CLIENT_INSECURE_SKIP_VERIFY":           "false",
			"AWS_LOCAL_CONFIGURATION":                    "/mnt/ceauthconfig",
			"MAX_HEAP":                                   "96",
			"ENFORCEMENT_CLIENT_CERTIFICATE_HEADER_NAME": "X-SSL-CERTIFICATE",
		})},
	})
}

func TestSyncLambda(t *testing.T) {
	stack, template := synth(t, validProps())

	template.HasResourceProperties(jsii.String(lambdaType), obj{
		"Handler":                      "bootstrap",
		"Runtime":                      "provided.al2023",
		"MemorySize":                   128,
		"Timeout":                      30,
		"ReservedConcurrentExecutions": 1,
		"Code": obj{
			"S3Bucket": s3BucketPerRegion("cloudentity-aws-api-gateway-authorizer"),
			"S3Key":    "cloudentity-aws-authorizer-v2-sync-2.22.0.zip",
		},
		"Environment": obj{"Variables": assertions.Match_ObjectEquals(&obj{
			"ACP_CLIENT_ID":                    "client-id",
			"ACP_CLIENT_SECRET":                "client-secret",
			"ACP_ISSUER_URL":                   "https://acp.example.com/default/default",
			"LOGGING_LEVEL":                    "info",
			"ANALYTICS_ENABLED":                "true",
			"HTTP_CLIENT_ROOT_CA":              "",
			"HTTP_CLIENT_INSECURE_SKIP_VERIFY": "false",
			"AWS_LOCAL_CONFIGURATION":          "/mnt/ceauthconfig",
			"AWS_AUTHORIZER_ARN":               getAtt(constructLogicalID(stack.AuthorizerLambda), "Arn"),
			"AWS_CREATE_AUTHORIZER":            "true",
			"AWS_AUTHORIZER_NAME":              "",
			"MAX_HEAP":                         "96",
		})},
	})
}

func TestLambdaEnvFollowsProps(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*StackProps)
		want  obj
	}{
		{"logging level", func(p *StackProps) { p.LoggingLevel = "debug" }, obj{"LOGGING_LEVEL": "debug"}},
		{"analytics disabled", func(p *StackProps) { p.AnalyticsDisabled = true }, obj{"ANALYTICS_ENABLED": "false"}},
		{"insecure skip verify", func(p *StackProps) { p.HTTPClientInsecureSkipVerify = true }, obj{"HTTP_CLIENT_INSECURE_SKIP_VERIFY": "true"}},
		{"root ca", func(p *StackProps) { p.HTTPClientRootCA = "-----BEGIN CERTIFICATE-----" }, obj{"HTTP_CLIENT_ROOT_CA": "-----BEGIN CERTIFICATE-----"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			props := validProps()
			tt.setup(&props)
			_, template := synth(t, props)

			template.ResourceCountIs(jsii.String(lambdaType), jsii.Number(2))
			template.AllResourcesProperties(jsii.String(lambdaType), obj{"Environment": obj{"Variables": tt.want}})
		})
	}
}

func TestAuthorizerNameOverride(t *testing.T) {
	props := validProps()
	props.AuthorizerName = "custom-authorizer"
	_, template := synth(t, props)

	template.HasResourceProperties(jsii.String(lambdaType), obj{
		"Environment": obj{"Variables": obj{"AWS_AUTHORIZER_NAME": "custom-authorizer"}},
	})
}

func TestManuallyCreateAuthorizer(t *testing.T) {
	autoBindStatements := arr{
		obj{"Action": "lambda:AddPermission", "Effect": "Allow", "Resource": "*"},
		obj{"Action": "apigateway:PATCH", "Effect": "Allow", "Resource": "arn:aws:apigateway:*::/restapis/*/resources/*/methods/*"},
		obj{"Action": "apigateway:POST", "Effect": "Allow", "Resource": "arn:aws:apigateway:*::/restapis/*/authorizers"},
	}
	readStatement := obj{
		"Action": "apigateway:GET",
		"Effect": "Allow",
		"Resource": arr{
			"arn:aws:apigateway:*::/restapis/*/deployments/*",
			"arn:aws:apigateway:*::/restapis/*/resources",
			"arn:aws:apigateway:*::/restapis/*/authorizers",
			"arn:aws:apigateway:*::/restapis/*/stages",
			"arn:aws:apigateway:*::/restapis",
		},
	}

	tests := []struct {
		name           string
		manual         bool
		wantCreateEnv  string
		wantStatements arr
	}{
		{"auto bind", false, "true", append(arr{readStatement}, autoBindStatements...)},
		{"manual", true, "false", arr{readStatement}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			props := validProps()
			props.ManuallyCreateAuthorizer = tt.manual
			stack, template := synth(t, props)

			template.HasResourceProperties(jsii.String(lambdaType), obj{
				"Environment": obj{"Variables": obj{"AWS_CREATE_AUTHORIZER": tt.wantCreateEnv}},
			})
			template.HasResourceProperties(jsii.String(policyType), obj{
				"PolicyDocument": obj{"Statement": tt.wantStatements},
				"Roles":          arr{obj{"Ref": constructLogicalID(syncLambdaRole(stack))}},
			})
		})
	}
}

func syncLambdaRole(stack Stack) awscdk.IResource {
	return syncLambda(stack).Node().FindChild(jsii.String("ServiceRole")).(awscdk.IResource)
}

func TestLambdaCode(t *testing.T) {
	assetCode := obj{
		"S3Bucket": assertions.Match_AnyValue(),
		"S3Key":    assertions.Match_StringLikeRegexp(jsii.String(`^[0-9a-f]{64}\.zip$`)),
	}
	localProps := validProps()
	localProps.AuthorizerZip = emptyZip(t, "authorizer.zip")
	localProps.SyncZip = emptyZip(t, "sync.zip")
	customS3Props := validProps()
	customS3Props.S3BucketName = "my-bucket"
	customS3Props.S3AuthorizerPrefix = "auth-"
	customS3Props.S3SyncPrefix = "sync-"

	tests := []struct {
		name                     string
		props                    StackProps
		authorizerCode, syncCode obj
	}{
		{
			name:           "custom bucket and prefixes",
			props:          customS3Props,
			authorizerCode: obj{"S3Bucket": s3BucketPerRegion("my-bucket"), "S3Key": "auth-2.22.0.zip"},
			syncCode:       obj{"S3Bucket": s3BucketPerRegion("my-bucket"), "S3Key": "sync-2.22.0.zip"},
		},
		{
			name:           "region resolved from env",
			props:          withEnv(validProps()),
			authorizerCode: obj{"S3Bucket": "cloudentity-aws-api-gateway-authorizer-us-east-1"},
			syncCode:       obj{"S3Bucket": "cloudentity-aws-api-gateway-authorizer-us-east-1"},
		},
		{
			name:           "local zips uploaded as assets",
			props:          localProps,
			authorizerCode: assetCode,
			syncCode:       assetCode,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, template := synth(t, tt.props)

			template.HasResourceProperties(jsii.String(lambdaType), obj{"Timeout": 10, "Code": tt.authorizerCode})
			template.HasResourceProperties(jsii.String(lambdaType), obj{"Timeout": 30, "Code": tt.syncCode})
		})
	}
}

func TestEFS(t *testing.T) {
	_, template := synth(t, validProps())

	fileSystemID := logicalID(t, template, "AWS::EFS::FileSystem", nil)
	template.HasResourceProperties(jsii.String("AWS::EFS::AccessPoint"), obj{
		"FileSystemId": obj{"Ref": fileSystemID},
		"PosixUser":    obj{"Uid": "1001", "Gid": "1001"},
		"RootDirectory": obj{
			"Path":         "/ceauthconfig",
			"CreationInfo": obj{"OwnerUid": "1001", "OwnerGid": "1001", "Permissions": "750"},
		},
	})
	template.AllResourcesProperties(jsii.String("AWS::EFS::MountTarget"), obj{"FileSystemId": obj{"Ref": fileSystemID}})

	accessPointID := logicalID(t, template, "AWS::EFS::AccessPoint", nil)
	template.AllResourcesProperties(jsii.String(lambdaType), obj{
		"FileSystemConfigs": arr{obj{
			"Arn":            obj{"Fn::Join": arr{"", assertions.Match_ArrayWith(&arr{obj{"Ref": accessPointID}})}},
			"LocalMountPath": "/mnt/ceauthconfig",
		}},
	})
}

func TestRemovalPolicies(t *testing.T) {
	_, template := synth(t, validProps())

	for _, resourceType := range []string{
		"AWS::EC2::VPC",
		"AWS::EFS::FileSystem",
		"AWS::EFS::AccessPoint",
		queueType,
		stateMachineType,
		ruleType,
	} {
		t.Run(resourceType, func(t *testing.T) {
			all := len(*template.FindResources(jsii.String(resourceType), nil))
			deleted := len(*template.FindResources(jsii.String(resourceType), obj{
				"DeletionPolicy":      "Delete",
				"UpdateReplacePolicy": "Delete",
			}))
			if all == 0 || deleted != all {
				t.Errorf("%d of %d %s resources have removal policy Delete", deleted, all, resourceType)
			}
		})
	}
}

func TestSubMinuteSyncTrigger(t *testing.T) {
	stack, template := synth(t, validProps())

	deadLetterQueueID := logicalID(t, template, queueType, obj{"Properties": obj{"MessageRetentionPeriod": 60}})
	template.HasResourceProperties(jsii.String(queueType), obj{
		"VisibilityTimeout": 30,
		"RedrivePolicy": obj{
			"deadLetterTargetArn": getAtt(deadLetterQueueID, "Arn"),
			"maxReceiveCount":     1,
		},
	})

	queueID := logicalID(t, template, queueType, obj{"Properties": obj{"VisibilityTimeout": 30}})
	template.HasResourceProperties(jsii.String("AWS::Lambda::EventSourceMapping"), obj{
		"BatchSize":      1,
		"EventSourceArn": getAtt(queueID, "Arn"),
		"FunctionName":   obj{"Ref": constructLogicalID(syncLambda(stack))},
	})

	stateMachineID := logicalID(t, template, stateMachineType, nil)
	template.HasResourceProperties(jsii.String(ruleType), obj{
		"ScheduleExpression": "rate(1 minute)",
		"State":              "ENABLED",
		"Targets":            arr{obj{"Arn": obj{"Ref": stateMachineID}}},
	})
}

func syncLambda(stack Stack) awscdk.IResource {
	return awscdk.Stack_Of(stack.AuthorizerLambda).Node().FindChild(jsii.String("SyncLambda")).(awscdk.IResource)
}

func TestSyncLooperFansOutReloadInterval(t *testing.T) {
	tests := []struct {
		interval  time.Duration
		wantTasks int
	}{
		{5 * time.Second, 12},
		{10 * time.Second, 6},
		{20 * time.Second, 3},
		{30 * time.Second, 2},
	}
	for _, tt := range tests {
		t.Run(tt.interval.String(), func(t *testing.T) {
			props := validProps()
			props.ReloadInterval = tt.interval
			_, template := synth(t, props)

			definition := stateMachineDefinition(t, template)
			if got := strings.Count(definition, `"DelaySeconds":`); got != tt.wantTasks {
				t.Errorf("got %d send-message tasks, want %d", got, tt.wantTasks)
			}
			lastDelay := regexp.MustCompile(fmt.Sprintf(`"DelaySeconds":%d\b`, 60-int(tt.interval.Seconds())))
			if !lastDelay.MatchString(definition) {
				t.Errorf("definition lacks last task %s", lastDelay)
			}
		})
	}
}

func stateMachineDefinition(t *testing.T, template assertions.Template) string {
	t.Helper()
	var sb strings.Builder
	for _, resource := range *template.FindResources(jsii.String(stateMachineType), nil) {
		parts := (*resource)["Properties"].(obj)["DefinitionString"].(obj)["Fn::Join"].(arr)[1].(arr)
		for _, part := range parts {
			if s, ok := part.(string); ok {
				sb.WriteString(s)
			}
		}
	}
	return sb.String()
}

func TestDirectSyncTriggerAtOneMinute(t *testing.T) {
	props := validProps()
	props.ReloadInterval = time.Minute
	stack, template := synth(t, props)

	template.ResourceCountIs(jsii.String(queueType), jsii.Number(0))
	template.ResourceCountIs(jsii.String(stateMachineType), jsii.Number(0))
	template.ResourceCountIs(jsii.String("AWS::Lambda::EventSourceMapping"), jsii.Number(0))

	syncLambdaID := constructLogicalID(syncLambda(stack))
	template.HasResourceProperties(jsii.String(ruleType), obj{
		"ScheduleExpression": "rate(1 minute)",
		"Targets":            arr{obj{"Arn": getAtt(syncLambdaID, "Arn")}},
	})
	template.HasResourceProperties(jsii.String("AWS::Lambda::Permission"), obj{
		"Action":       "lambda:InvokeFunction",
		"Principal":    "events.amazonaws.com",
		"FunctionName": getAtt(syncLambdaID, "Arn"),
	})
}

func TestVpc(t *testing.T) {
	lookupProps := withEnv(validProps())
	lookupProps.VpcID = "vpc-0123456789abcdef0"

	tests := []struct {
		name     string
		props    StackProps
		wantVpcs int
	}{
		{"created when VpcID empty", validProps(), 1},
		{"looked up when VpcID set", lookupProps, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, template := synth(t, tt.props)

			template.ResourceCountIs(jsii.String("AWS::EC2::VPC"), jsii.Number(tt.wantVpcs))
		})
	}
}

func TestPropsValidation(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*StackProps)
		wantErr string
	}{
		{"missing client id", func(p *StackProps) { p.ClientID = "" }, "'ClientID' failed on the 'required'"},
		{"missing client secret", func(p *StackProps) { p.ClientSecret = "" }, "'ClientSecret' failed on the 'required'"},
		{"missing issuer url", func(p *StackProps) { p.IssuerURL = "" }, "'IssuerURL' failed on the 'required'"},
		{"missing version", func(p *StackProps) { p.Version = "" }, "'Version' failed on the 'required'"},
		{"unknown logging level", func(p *StackProps) { p.LoggingLevel = "trace" }, "'LoggingLevel' failed on the 'oneof'"},
		{"reload interval below 5s", func(p *StackProps) { p.ReloadInterval = time.Second }, "'ReloadInterval' failed on the 'min'"},
		{"reload interval above 1m", func(p *StackProps) { p.ReloadInterval = 2 * time.Minute }, "'ReloadInterval' failed on the 'max'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			props := validProps()
			tt.setup(&props)

			_, err := NewStack(awscdk.NewApp(nil), "Test", props)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("got error %v, want one containing %q", err, tt.wantErr)
			}
		})
	}
}
