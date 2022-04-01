package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/pkg/errors"
)

const EC2_CLASSIC_NAMESERVER = "172.16.0.23"

func main() {
	var file string
	var helpShort, helpLong bool

	flags := flag.NewFlagSet("ec2-env", flag.ContinueOnError)
	flags.StringVar(&file, "file", "", "Write environment variables to a file.")
	flags.BoolVar(&helpShort, "h", false, "Display usage.")
	flags.BoolVar(&helpLong, "help", false, "Display usage.")
	err := flags.Parse(os.Args[1:])

	if helpShort || helpLong {
		usage()
		os.Exit(0)
	} else if err != nil || flags.NArg() > 0 {
		usage()
		os.Exit(1)
	}

	var out io.Writer = os.Stdout
	if file != "" {
		out, err = os.Create(file)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	if err := writeEnv(out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// usage displays program usage info
func usage() {
	fmt.Fprintln(os.Stderr, "Export EC2 metadata to environment variables.")
	fmt.Fprintln(os.Stderr, "usage: ec2-env [-file=FILE]")
}

type prop struct {
	name  string
	value interface{}
}

// writeEnv generated environment variables and writes them to `out`.
func writeEnv(out io.Writer) error {
	props := []prop{}

	session := session.New()

	metadataSvc := ec2metadata.New(session, &aws.Config{
		Region: aws.String("us-east-1"),
	})
	if !metadataSvc.Available() {
		return errors.New("not running on an ec2 instance")
	}
	props = append(props, collectMetadataProps(metadataSvc)...)

	identity, err := metadataSvc.GetInstanceIdentityDocument()
	if err != nil {
		return errors.Wrap(err, "failed to retrieve instance identity")
	}
	props = append(props, collectIdentityDocumentProps(&identity)...)

	ec2Svc := ec2.New(session, &aws.Config{
		Region: aws.String(identity.Region),
	})
	res, err := ec2Svc.DescribeInstances(&ec2.DescribeInstancesInput{
		InstanceIds: aws.StringSlice([]string{identity.InstanceID}),
	})
	if err != nil {
		return errors.Wrap(err, "failed to describe instance ")
	}
	if len(res.Reservations) == 0 {
		return errors.Errorf("reservations for instance %s not found", identity.InstanceID)
	}
	if len(res.Reservations[0].Instances) == 0 {
		return errors.Errorf("instance %s not found", identity.InstanceID)
	}
	instance := res.Reservations[0].Instances[0]

	props = append(props, collectInstanceProps(instance)...)

	sort.Slice(props, func(i, j int) bool { return props[i].name < props[j].name })
	for _, prop := range props {
		fmt.Fprintf(out, "%s=%s\n", prop.name, prop.value)
	}

	return nil
}

func collectMetadataProps(metadataSvc *ec2metadata.EC2Metadata) []prop {
	return []prop{
		{name: "EC2_AVAILABILITY_ZONE_ID", value: getAvailabilityZoneId(metadataSvc)},
		{name: "EC2_NAMESERVER", value: getNameserver(metadataSvc)},
	}
}

func getNameserver(metadataSvc *ec2metadata.EC2Metadata) string {
	nameserver := EC2_CLASSIC_NAMESERVER
	mac, err := metadataSvc.GetMetadata("mac")
	if err != nil {
		return nameserver
	}
	path := fmt.Sprintf("network/interfaces/macs/%s/vpc-ipv4-cidr-block", mac)
	cidr, err := metadataSvc.GetMetadata(path)
	if err != nil {
		return nameserver
	}
	ip, _, err := net.ParseCIDR(cidr)
	if err != nil {
		return nameserver
	}
	ip[len(ip)-1] = ip[len(ip)-1] + 2
	return ip.String()
}

func getAvailabilityZoneId(metadataSvc *ec2metadata.EC2Metadata) string {
	path := "placement/availability-zone-id"
	az_id, err := metadataSvc.GetMetadata(path)
	if err != nil {
		return ""
	}
	return az_id
}

func collectIdentityDocumentProps(identity *ec2metadata.EC2InstanceIdentityDocument) []prop {
	return []prop{
		{name: "AWS_DEFAULT_REGION_SHORT", value: shortRegion(identity.Region)},
		{name: "AWS_DEFAULT_REGION", value: identity.Region},
		{name: "AWS_REGION_SHORT", value: shortRegion(identity.Region)},
		{name: "AWS_REGION", value: identity.Region},
		{name: "EC2_ACCOUNT_ID", value: identity.AccountID},
		{name: "EC2_AVAILABILITY_ZONE_LETTER", value: zoneLetter(identity.AvailabilityZone)},
		{name: "EC2_AVAILABILITY_ZONE", value: identity.AvailabilityZone},
		{name: "EC2_IMAGE_ID", value: identity.ImageID},
		{name: "EC2_INSTANCE_ID", value: identity.InstanceID},
		{name: "EC2_INSTANCE_TYPE", value: identity.InstanceType},
		{name: "EC2_REGION_SHORT", value: shortRegion(identity.Region)},
		{name: "EC2_REGION", value: identity.Region},
	}
}

// shortRegion returns a three letter abbreviation for the region.
func shortRegion(region string) string {
	buf := bytes.Buffer{}
	for _, part := range strings.Split(region, "-") {
		if len(part) > 0 {
			buf.WriteString(part[:1])
		}
	}
	return buf.String()
}

// zoneLetter returns a single letter representing the availability zone.
func zoneLetter(zone string) string {
	if len(zone) > 0 {
		return zone[len(zone)-1:]
	}
	return ""
}

// tagEnvName converts EC2 tag names to environment variable names.
func tagEnvName(name string) string {
	// Clean up disallowed characters.
	name = regexp.MustCompile(`[^a-zA-Z0-9]`).ReplaceAllString(name, "_")
	name = regexp.MustCompile(`_+`).ReplaceAllString(name, "_")

	// Convert to standard case format.
	if strings.Contains(name, "_") {
		name = strings.ToUpper(name)
	} else {
		name = toSnake(name)
	}
	return name
}

// toSnake converts a string in CamelCase or pascalCase to snake_case.
func toSnake(name string) string {
	buf := bytes.Buffer{}
	for i := 0; i < len(name); i++ {
		c := rune(name[i])
		if unicode.IsUpper(c) {
			if i > 0 && (i+1 >= len(name) || unicode.IsLower(rune(name[i+1]))) {
				buf.WriteRune('_')
			}
			buf.WriteRune(c)
		} else {
			buf.WriteRune(unicode.ToUpper(c))
		}
	}
	return buf.String()
}

func collectInstanceProps(instance *ec2.Instance) []prop {
	props := []prop{
		{name: "EC2_KEYNAME", value: aws.StringValue(instance.KeyName)},
		{name: "EC2_PRIVATE_DNS", value: aws.StringValue(instance.PrivateDnsName)},
		{name: "EC2_PRIVATE_IP", value: aws.StringValue(instance.PrivateIpAddress)},
		{name: "EC2_PUBLIC_DNS", value: aws.StringValue(instance.PublicDnsName)},
		{name: "EC2_PUBLIC_IP", value: aws.StringValue(instance.PublicIpAddress)},
		{name: "EC2_SUBNET_ID", value: aws.StringValue(instance.SubnetId)},
		{name: "EC2_VPC_ID", value: aws.StringValue(instance.VpcId)},
	}
	for _, tag := range instance.Tags {
		name := fmt.Sprintf("EC2_TAG_%s", tagEnvName(aws.StringValue(tag.Key)))
		value := aws.StringValue(tag.Value)
		props = append(props, prop{name, value})
	}
	return props
}
