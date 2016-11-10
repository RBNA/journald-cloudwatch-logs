package main

import (
	"fmt"
	"github.com/aws/aws-sdk-go/service/ec2"
	awsSession "github.com/aws/aws-sdk-go/aws/session"
	 "github.com/aws/aws-sdk-go/aws"
)

func main() {


	fmt.Println("Started")

	var instanceId = "i-01d5db425dc49f060"

	var config = aws.Config{Region: aws.String("us-west-2")}

	sess, err := awsSession.NewSession(&config)

	fmt.Println("created session")


	if err != nil {
		fmt.Println("failed to create session,", err)
		return
	}


	fmt.Println("Creating new instance")

	svc := ec2.New(sess)

	params := &ec2.DescribeInstancesInput{
		InstanceIds: []*string{
			aws.String(instanceId), // Required
			// More values...
		},
	}


	fmt.Println("Running describe")
	resp, err := svc.DescribeInstances(params)

	if err != nil {
		// Print the error, cast err to awserr.Error to get the Code and
		// Message from an error.
		fmt.Println(err.Error())
		return
	}


	fmt.Println("DONE")

	if len(resp.Reservations) > 0 && len(resp.Reservations[0].Instances) > 0 {
		var instance = resp.Reservations[0].Instances[0]
		fmt.Println(instance)
	} else {
		fmt.Println("Not found")
	}

}
