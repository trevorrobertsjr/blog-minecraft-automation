package main

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
)

// Event represents the incoming event with an action
type Event struct {
	Action string `json:"action"`
}

// Handler handles the incoming Lambda event
func Handler(ctx context.Context, event Event) (string, error) {
	instanceID := os.Getenv("INSTANCE_ID")
	if instanceID == "" {
		return "", fmt.Errorf("INSTANCE_ID environment variable is not set")
	}

	instanceIDs := []string{instanceID}

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return "", fmt.Errorf("unable to load SDK config, %v", err)
	}

	svc := ec2.NewFromConfig(cfg)

	switch event.Action {
	case "start":
		_, err := svc.StartInstances(ctx, &ec2.StartInstancesInput{
			InstanceIds: instanceIDs,
		})
		if err != nil {
			return "", fmt.Errorf("failed to start instance: %v", err)
		}
		return fmt.Sprintf("Started instance %s", instanceID), nil
	case "stop":
		_, err := svc.StopInstances(ctx, &ec2.StopInstancesInput{
			InstanceIds: instanceIDs,
		})
		if err != nil {
			return "", fmt.Errorf("failed to stop instance: %v", err)
		}
		return fmt.Sprintf("Stopped instance %s", instanceID), nil
	default:
		return "", fmt.Errorf("unknown action: %s", event.Action)
	}
}

func main() {
	lambda.Start(Handler)
}
