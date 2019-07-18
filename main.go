package main

import (
	"fmt"
	"log"
	"os/exec"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
)

func main() {
	instanceID, instanceRegion, ipAddr, err := retrieveInstanceMeta()
	errHandle(err)
	tags := aws.StringValueMap(getTags(instanceID, instanceRegion))

	parsedHostname := hostnameParser(instanceID, instanceRegion, tags)

	if isASG(tags) {
		fmt.Printf("Hostname: %v\n\n", parsedHostname)
		setHostname(parsedHostname)
		fileWriter(ipAddr, parsedHostname)
		setAWSTag(instanceID, instanceRegion, parsedHostname)

	} else {
		fmt.Printf("Hostname: %v\n\n", parsedHostname)
		setHostname(parsedHostname)
		fileWriter(ipAddr, parsedHostname)
	}
}

func retrieveInstanceMeta() (string, string, string, error) {
	sess, err := session.NewSession()
	svc := ec2metadata.New(sess)

	instanceID, _ := svc.GetMetadata("instance-id")
	region, _ := svc.Region()
	ipAddr, _ := svc.GetMetadata("local-ipv4")

	return instanceID, region, ipAddr, err

}

func isASG(tags map[string]string) bool {
	for k, _ := range tags {
		if k == "aws:autoscaling:groupName" {
			return true
		}
	}
	return false
}

func getTags(instanceID, instanceRegion string) map[string]*string {

	tagMaps := map[string]*string{}

	svc := ec2.New(session.New(&aws.Config{
		Region: aws.String(instanceRegion),
	}))
	input := &ec2.DescribeInstancesInput{
		InstanceIds: []*string{
			aws.String(instanceID),
		},
	}

	result, err := svc.DescribeInstances(input)

	errHandle(err)

	for _, reservation := range result.Reservations {
		for _, tag := range reservation.Instances {
			for _, value := range tag.Tags {
				tagMaps[aws.StringValue(value.Key)] = value.Value
			}
		}
	}
	return tagMaps
}

func hostnameParser(instanceID, region string, tagMaps map[string]string) string {
	var env, role, parsedHostname string

	regions := map[string]string{
		"us-east-1": "use1",
		"us-east-2": "use2",
		"us-west-1": "usw1",
		"us-west-2": "usw2",
	}

	parsedRegion, _ := regions[region]
	parsedInstanceID := strings.Trim(instanceID, "i-")

	if isASG(tagMaps) {
		for key, value := range tagMaps {
			if key == "Environment" {
				env = value
			} else if key == "Role" {
				role = value
			}
		}
		passedRole := strings.Replace(role, "_", "-", -1)
		parsedHostname = fmt.Sprintf("%v-aws-%v-%v-%v", env, parsedRegion, passedRole, parsedInstanceID)
	} else {
		for key, value := range tagMaps {
			if key == "Name" {
				parsedHostname = value
			}
		}
	}

	return parsedHostname
}

func setHostname(hostname string) {
	commandline := fmt.Sprintf("hostnamectl --static --no-ask-password set-hostname %s", hostname)
	cmd := exec.Command("/bin/bash", "-c", commandline)
	fmt.Println("Setting Hostname...")
	err := cmd.Run()
	errHandle(err)
}

// Revisit in future
func fileWriter(ipAddress, hostName string) {

	fmt.Println("Writing /etc/hosts for ip address")
	ipAddrCommandString := fmt.Sprintf("sed -i -n -e '/^'%v'.*/!p' -e '$a'%v' '%v'' /etc/hosts", ipAddress, ipAddress, hostName)
	fmt.Println("Overriding 127.0.0.1 in /etc/hosts")
	localhostCommandString := fmt.Sprintf("sed -i 's/^127.0.0.1.*/127.0.0.1 localhost localhost.localdomain localhost4 localhost4.localdomain4/g' /etc/hosts")
	localHostsCMD := exec.Command("/bin/bash", "-c", localhostCommandString)
	ipAddrCMD := exec.Command("/bin/bash", "-c", ipAddrCommandString)

	fmt.Println("Setting hostname persists to true")
	persistHostnameCommandString := fmt.Sprintf("echo preserve_hostname: true > /etc/cloud/cloud.cfg.d/09_hostname.cfg")
	persistHostnameCMD := exec.Command("/bin/bash", "-c", persistHostnameCommandString)

	localHostsCMD.Run()
	ipAddrCMD.Run()
	persistHostnameCMD.Run()
}

func setAWSTag(instanceID, instanceRegion, hostname string) {
	svc := ec2.New(session.New(&aws.Config{
		Region: aws.String(instanceRegion),
	}))

	input := &ec2.CreateTagsInput{
		Resources: []*string{
			aws.String(instanceID),
		},
		Tags: []*ec2.Tag{
			{
				Key:   aws.String("Name"),
				Value: aws.String(hostname),
			},
		},
	}

	result, err := svc.CreateTags(input)
	fmt.Println("Tagging EC2 Instance...")
	fmt.Println(result)
	errHandle(err)
}

func errHandle(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
