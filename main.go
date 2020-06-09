package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"regexp"
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

	flags := flag.NewFlagSet("ec20env", flag.ContinueOnError)
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

// writeEnv generated environment variables and writes them to `out`.
func writeEnv(out io.Writer) error {
	metaSvc := ec2metadata.New(session.New(), &aws.Config{
		Region: aws.String("us-east-1"),
	})
	if !metaSvc.Available() {
		return errors.New("not running on an ec2 instance")
	}

	identity, err := metaSvc.GetInstanceIdentityDocument()
	if err != nil {
		return errors.Wrap(err, "failed to retrieve instance identity")
	}

	ec2Svc := ec2.New(session.New(), &aws.Config{
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

	wr := func(name string, value interface{}) {
		fmt.Fprintf(out, "%s=%s\n", name, value)
	}

    wr("AWS_REGION", identity.AvailabilityZone)
    wr("AWS_REGION_SHORT", zoneLetter(identity.AvailabilityZone))
    wr("AWS_DEFAULT_REGION", identity.AvailabilityZone)
    wr("AWS_DEFAULT_REGION_SHORT", zoneLetter(identity.AvailabilityZone))

	wr("EC2_AVAILABILITY_ZONE", identity.AvailabilityZone)
	wr("EC2_AVAILABILITY_ZONE_LETTER", zoneLetter(identity.AvailabilityZone))
	wr("EC2_REGION", identity.Region)
	wr("EC2_REGION_SHORT", shortRegion(identity.Region))
	wr("EC2_INSTANCE_ID", identity.InstanceID)
	wr("EC2_INSTANCE_TYPE", identity.InstanceType)
	wr("EC2_ACCOUNT_ID", identity.AccountID)
	wr("EC2_IMAGE_ID", identity.ImageID)

	wr("EC2_PRIVATE_DNS", aws.StringValue(instance.PrivateDnsName))
	wr("EC2_PRIVATE_IP", aws.StringValue(instance.PrivateIpAddress))
	wr("EC2_PUBLIC_DNS", aws.StringValue(instance.PublicDnsName))
	wr("EC2_PUBLIC_IP", aws.StringValue(instance.PublicIpAddress))
	wr("EC2_SUBNET_ID", aws.StringValue(instance.SubnetId))
	wr("EC2_VPC_ID", aws.StringValue(instance.VpcId))
	wr("EC2_KEYNAME", aws.StringValue(instance.KeyName))
	wr("EC2_NAMESERVER", getNameserver(metaSvc))

	for _, tag := range instance.Tags {
		name := fmt.Sprintf("EC2_TAG_%s", tagEnvName(aws.StringValue(tag.Key)))
		wr(name, aws.StringValue(tag.Value))
	}
	return nil
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

func getNameserver(metaSvc *ec2metadata.EC2Metadata) string {
	nameserver := EC2_CLASSIC_NAMESERVER
	mac, err := metaSvc.GetMetadata("mac")
	if err != nil {
		return nameserver
	}
	path := fmt.Sprintf("network/interfaces/macs/%s/vpc-ipv4-cidr-block", mac)
	cidr, err := metaSvc.GetMetadata(path)
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
