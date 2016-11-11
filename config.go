package main

import (
	"fmt"
	"io/ioutil"

	"github.com/aws/aws-sdk-go/aws"
	awsCredentials "github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/credentials/ec2rolecreds"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	awsSession "github.com/aws/aws-sdk-go/aws/session"

	"github.com/hashicorp/hcl"
	"github.com/aws/aws-sdk-go/service/ec2"
	"errors"
)

const DEV_DEBUG  = true
const TEST_INSTANCE_ID = "i-01d5db425dc49f060"


type Config struct {
	AWSCredentials *awsCredentials.Credentials
	AWSRegion      string
	EC2InstanceId  string
	LogGroupName   string
	LogStreamName  string
	LogPriority    Priority
	StateFilename  string
	JournalDir     string
	BufferSize     int
}

type fileConfig struct {
	AWSRegion     string `hcl:"aws_region"`
	EC2InstanceId string `hcl:"ec2_instance_id"`
	LogGroupName  string `hcl:"log_group"`
	LogStreamName string `hcl:"log_stream"`
	LogPriority   string `hcl:"log_priority"`
	StateFilename string `hcl:"state_file"`
	JournalDir    string `hcl:"journal_dir"`
	BufferSize    int    `hcl:"buffer_size"`
}

func getLogLevel(priority string) (Priority, error) {

	logLevels := map[Priority][]string{
		EMERGENCY: {"0", "emerg"},
		ALERT:     {"1", "alert"},
		CRITICAL:  {"2", "crit"},
		ERROR:     {"3", "err"},
		WARNING:   {"4", "warning"},
		NOTICE:    {"5", "notice"},
		INFO:      {"6", "info"},
		DEBUG:     {"7", "debug"},
	}

	for i, s := range logLevels {
		if s[0] == priority || s[1] == priority {
			return i, nil
		}
	}

	return DEBUG, fmt.Errorf("'%s' is unsupported log priority", priority)
}

func LoadConfig(filename string) (*Config, error) {
	configBytes, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	config := &Config{}
	var session = awsSession.New(&aws.Config{})
	metaClient := ec2metadata.New(session)

	var fConfig fileConfig
	err = hcl.Decode(&fConfig, string(configBytes))
	if err != nil {
		return nil, err
	}

	if fConfig.LogGroupName == "" {
		config.LogGroupName, err = metaClient.GetMetadata("security-groups")

		if (err!=nil) {
			config.LogGroupName = "log_group"
			fmt.Printf("Log group name was not set and can't get from AWS EC2 Meta, using default %s \n",
				config.LogGroupName)
		} else {
			fmt.Printf("Log group name set to security group name %s \n",
				config.LogGroupName)

		}
	} else {
		config.LogGroupName = fConfig.LogGroupName
	}
	if fConfig.StateFilename == "" {
		return nil, errors.New("state_file is required")
	}



	if fConfig.AWSRegion != "" {
		config.AWSRegion = fConfig.AWSRegion
	} else {
		region, err := metaClient.Region()
		if err != nil {
			return nil, fmt.Errorf("unable to detect AWS region: %s", err)
		}
		config.AWSRegion = region
	}

	if fConfig.EC2InstanceId != "" {
		config.EC2InstanceId = fConfig.EC2InstanceId
	} else {
		instanceId, err := metaClient.GetMetadata("instance-id")
		if err != nil {
			return nil, fmt.Errorf("unable to detect EC2 instance id: %s", err)
		}
		config.EC2InstanceId = instanceId
	}

	if fConfig.LogPriority == "" {
		// Log everything
		config.LogPriority = DEBUG
	} else {
		config.LogPriority, err = getLogLevel(fConfig.LogPriority)
		if err != nil {
			return nil, fmt.Errorf("The provided log filtering '%s' is unsupported by systemd!", fConfig.LogPriority)
		}
	}


	if fConfig.LogStreamName != "" {
		config.LogStreamName = fConfig.LogStreamName
	} else {

		var instanceId, az, name string
		instanceId, err = FindInstanceId(metaClient)
		az, err = FindAZ(metaClient)
		name, err = FindInstanceName(instanceId, config.AWSRegion, session)
		config.LogStreamName = name+"-"+instanceId + "-" + az
		fmt.Printf("LogStreamName was not set so using %s \n", config.LogStreamName)
	}

	config.StateFilename = fConfig.StateFilename
	config.JournalDir = fConfig.JournalDir

	if fConfig.BufferSize != 0 {
		config.BufferSize = fConfig.BufferSize
	} else {
		config.BufferSize = 100
	}

	config.AWSCredentials = awsCredentials.NewChainCredentials([]awsCredentials.Provider{
		&awsCredentials.EnvProvider{},
		&ec2rolecreds.EC2RoleProvider{
			Client: metaClient,
		},
	})

	return config, nil
}

func (c *Config) NewAWSSession() *awsSession.Session {
	config := &aws.Config{
		Credentials: c.AWSCredentials,
		Region:      aws.String(c.AWSRegion),
		MaxRetries:  aws.Int(3),
	}
	return awsSession.New(config)
}


func FindInstanceId(metaClient *ec2metadata.EC2Metadata) (string, error) {

	instanceId, err := metaClient.GetMetadata("instance-id")

	if err != nil {
		if DEV_DEBUG {
			return TEST_INSTANCE_ID, nil
		}else {
			return "", fmt.Errorf("unable to detect EC2 instance id: %s", err)
		}
	}
	return instanceId, nil
}

func FindAZ(metaClient *ec2metadata.EC2Metadata) (string, error) {

	az, err := metaClient.GetMetadata("placement/availability-zone")

	if err != nil {
		if DEV_DEBUG {
			return "NO_AZ", nil
		}else {
			return "", fmt.Errorf("unable to detect EC2 az id: %s", err)
		}
	}
	return az, nil
}




func FindInstanceName(instanceId string, region string, session *awsSession.Session) (string, error) {

	var name = "NO_NAME"
	var err error

	ec2Service := ec2.New(session, aws.NewConfig().WithRegion(region))

	params := &ec2.DescribeInstancesInput{
		InstanceIds: []*string{
			aws.String(instanceId), // Required
			// More values...
		},
	}

	resp, err := ec2Service.DescribeInstances(params)

	if err != nil {
		fmt.Println(err)
		return name, err
	}

	if len(resp.Reservations) > 0 && len(resp.Reservations[0].Instances) > 0 {
		var instance = resp.Reservations[0].Instances[0]
		if len (instance.Tags) > 0 {

			for _, tag := range instance.Tags {
				if *tag.Key == "Name" {
					return *tag.Value, err
				}
				fmt.Println("KEY " + *tag.Key)

			}
		}
		return name, errors.New("Could not find tag")

	} else {
		return name, errors.New("Could not find reservation")
	}
}