package demo

import (
	"os"
	"testing"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/assertions"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/jsii-runtime-go"
)

type (
	obj = map[string]interface{}
	arr = []interface{}
)

func TestMain(m *testing.M) {
	code := m.Run()
	jsii.Close()
	os.Exit(code)
}

func synth(t *testing.T) (assertions.Template, string) {
	t.Helper()
	app := awscdk.NewApp(nil)
	authorizerStack := awscdk.NewStack(app, jsii.String("Authorizer"), nil)
	authorizerLambda := awslambda.NewFunction(authorizerStack, jsii.String("AuthorizerLambda"), &awslambda.FunctionProps{
		Code:    awslambda.Code_FromInline(jsii.String("exports.handler = () => {}")),
		Handler: jsii.String("index.handler"),
		Runtime: awslambda.Runtime_NODEJS_20_X(),
	})

	stack, err := NewStack(app, "Demo", authorizerLambda, awscdk.StackProps{})
	if err != nil {
		t.Fatalf("NewStack: %v", err)
	}
	return assertions.Template_FromStack(stack, nil), exportedArn(t, authorizerStack, authorizerLambda)
}

func exportedArn(t *testing.T, stack awscdk.Stack, fn awslambda.Function) string {
	t.Helper()
	lambdaID := stack.GetLogicalId(fn.Node().DefaultChild().(awscdk.CfnElement))
	outputs := *assertions.Template_FromStack(stack, nil).FindOutputs(jsii.String("*"), obj{
		"Value": obj{"Fn::GetAtt": arr{*lambdaID, "Arn"}},
	})
	if len(outputs) != 1 {
		t.Fatalf("expected exactly one output exporting the lambda ARN, got %d", len(outputs))
	}
	for _, output := range outputs {
		return (*output)["Export"].(obj)["Name"].(string)
	}
	return ""
}

func singleLogicalID(t *testing.T, template assertions.Template, resourceType string) string {
	t.Helper()
	resources := *template.FindResources(jsii.String(resourceType), nil)
	if len(resources) != 1 {
		t.Fatalf("expected exactly one %s, got %d", resourceType, len(resources))
	}
	for id := range resources {
		return id
	}
	return ""
}

func TestRequestAuthorizerUsesAuthorizerLambda(t *testing.T) {
	template, authorizerArnExport := synth(t)

	template.HasResourceProperties(jsii.String("AWS::ApiGateway::Authorizer"), obj{
		"Type":           "REQUEST",
		"Name":           "CloudentityAWSAuthorizer",
		"IdentitySource": "method.request.header.Authorization",
		"RestApiId":      obj{"Ref": singleLogicalID(t, template, "AWS::ApiGateway::RestApi")},
		"AuthorizerUri": obj{"Fn::Join": arr{"", assertions.Match_ArrayWith(&arr{
			":lambda:path/2015-03-31/functions/",
			obj{"Fn::ImportValue": authorizerArnExport},
			"/invocations",
		})}},
	})
}

func TestRootMethodProtectedByAuthorizer(t *testing.T) {
	template, _ := synth(t)

	template.HasResourceProperties(jsii.String("AWS::ApiGateway::Method"), obj{
		"HttpMethod":        "GET",
		"AuthorizationType": "CUSTOM",
		"AuthorizerId":      obj{"Ref": singleLogicalID(t, template, "AWS::ApiGateway::Authorizer")},
		"Integration":       obj{"Type": "MOCK"},
	})
}

func TestApiAndStage(t *testing.T) {
	template, _ := synth(t)

	template.HasResourceProperties(jsii.String("AWS::ApiGateway::RestApi"), obj{"Name": "SampleAPI", "Description": "Sample API"})
	template.HasResourceProperties(jsii.String("AWS::ApiGateway::Stage"), obj{"StageName": "test"})
}

// Pinned gap: SetupPermissions (#7) is a no-op for a cross-stack imported ARN, so no invoke permission is emitted.
func TestNoInvokePermissionForImportedAuthorizer(t *testing.T) {
	template, _ := synth(t)

	template.ResourceCountIs(jsii.String("AWS::Lambda::Permission"), jsii.Number(0))
}
