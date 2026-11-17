package main

import (
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/jsii-runtime-go"
	"github.com/cloudentity/awsauthorizercdk/pkg/stacks/authorizer"
)

func TestMain(m *testing.M) {
	code := m.Run()
	jsii.Close()
	os.Exit(code)
}

func appWithContext(context map[string]interface{}) awscdk.App {
	return awscdk.NewApp(&awscdk.AppProps{Context: &context})
}

func TestReadStackProps(t *testing.T) {
	t.Setenv("ACP_CLIENT_SECRET", "secret-from-env")
	app := appWithContext(map[string]interface{}{
		"syncZip":                      "/lambdas/sync.zip",
		"authorizerZip":                "/lambdas/authorizer.zip",
		"manuallyCreateAuthorizer":     "true",
		"authorizerName":               "my-authorizer",
		"clientID":                     "client-id",
		"issuerURL":                    "https://acp.example.com/default/default",
		"vpcID":                        "vpc-0123456789abcdef0",
		"version":                      "2.22.0",
		"loggingLevel":                 "debug",
		"reloadInterval":               "15s",
		"analyticsDisabled":            "true",
		"injectContext":                "true",
		"enforcementAllowUnknown":      "true",
		"httpClientRootCA":             "-----BEGIN CERTIFICATE-----",
		"httpClientInsecureSkipVerify": "true",
		"s3BucketName":                 "my-bucket",
		"stackName":                    "my-stack",
	})

	var props authorizer.StackProps
	if err := readStackProps(app, &props); err != nil {
		t.Fatalf("readStackProps: %v", err)
	}

	want := authorizer.StackProps{
		StackProps:                   awscdk.StackProps{StackName: jsii.String("my-stack")},
		SyncZip:                      "/lambdas/sync.zip",
		AuthorizerZip:                "/lambdas/authorizer.zip",
		ManuallyCreateAuthorizer:     true,
		AuthorizerName:               "my-authorizer",
		ClientID:                     "client-id",
		ClientSecret:                 "secret-from-env",
		IssuerURL:                    "https://acp.example.com/default/default",
		VpcID:                        "vpc-0123456789abcdef0",
		Version:                      "2.22.0",
		LoggingLevel:                 "debug",
		ReloadInterval:               15 * time.Second,
		AnalyticsDisabled:            true,
		InjectContext:                true,
		EnforcementAllowUnknown:      true,
		HTTPClientRootCA:             "-----BEGIN CERTIFICATE-----",
		HTTPClientInsecureSkipVerify: true,
		S3BucketName:                 "my-bucket",
	}
	if !reflect.DeepEqual(props, want) {
		t.Errorf("got %+v\nwant %+v", props, want)
	}
}

func TestReadStackPropsReloadInterval(t *testing.T) {
	tests := []struct {
		value   string
		want    time.Duration
		wantErr bool
	}{
		{"", 0, false},
		{"1m", time.Minute, false},
		{"soon", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			var props authorizer.StackProps
			err := readStackProps(appWithContext(map[string]interface{}{"reloadInterval": tt.value}), &props)
			if (err != nil) != tt.wantErr {
				t.Fatalf("got error %v, wantErr %v", err, tt.wantErr)
			}
			if props.ReloadInterval != tt.want {
				t.Errorf("got %v, want %v", props.ReloadInterval, tt.want)
			}
		})
	}
}

func TestReadBoolCtxParam(t *testing.T) {
	tests := []struct {
		context map[string]interface{}
		want    bool
	}{
		{map[string]interface{}{"flag": "true"}, true},
		{map[string]interface{}{"flag": "True"}, false},
		{map[string]interface{}{"flag": "1"}, false},
		{map[string]interface{}{}, false},
	}
	for _, tt := range tests {
		if got := readBoolCtxParam(appWithContext(tt.context), "flag"); got != tt.want {
			t.Errorf("readBoolCtxParam(%v) = %v, want %v", tt.context, got, tt.want)
		}
	}
}
