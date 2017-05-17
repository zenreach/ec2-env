ec2-env
=======
This tool retrieves information about the EC2 instance that runs it and prints them formatted environment variables. The output can be eval'd by bash or written to a file with the `-file` option.

Usage
-----
Calling the tool will output environment variables to stdout:

    eval $(ec2-env)

Alternatively you can write an environment file:

    ec2-env -file=/etc/environment.ec2

This second usage is useful for integraiton with systemd unit files using the `EnvironmentFile` option.

Variables
---------
The following environment variables are written by `ec2-env`:

| Environment Variable Name      | Description                                         |
| ------------------------------ | --------------------------------------------------- |
| `EC2_AVAILABILITY_ZONE`        | AWS availability zone.                              |
| `EC2_AVAILABILITY_ZONE_LETTER` | The letter (a, b, c, etc) of the availability zone. |
| `EC2_REGION`                   | AWS region (us-east-1, etc).                        |
| `EC2_REGION_SHORT`             | A shortened region identifier.                      |
| `EC2_INSTANCE_ID`              | EC2 instance ID.                                    |
| `EC2_INSTANCE_TYPE`            | EC2 instance type.                                  |
| `EC2_ACCOUNT_ID`               | AWS Account ID.                                     |
| `EC2_IMAGE_ID`                 | EC2 instance AMI.                                   |
| `EC2_PRIVATE_DNS`              | EC2 instance private DNS name.                      |
| `EC2_PRIVATE_IP`               | EC2 instance private IP address.                    |
| `EC2_PUBLIC_DNS`               | EC2 instance public DNS name.                       |
| `EC2_PUBLIC_IP`                | EC2 instance public IP address.                     |
| `EC2_SUBNET_ID`                | ID of the subnet the instance is running in.        |
| `EC2_VPC_ID`                   | ID of the VPC the instance is running in.           |
| `EC2_KEYNAME`                  | EC2 key name.                                       |
| `EC2_NAMESERVER`               | EC2 nameserver assigned to the instance.            |
| `EC2_TAG_%s`                   | EC2 tags. These are templated. See Instance Tags.   |

Instance Tags
-------------
The instance's tags are also written to environment variables. Each tag is written to a variable of the form `EC2_TAG_%s` where `%s` is the sanitized tag name.

Tag names are sanitized by converting to uppercase and replacing non-alphanumeric characters to underscores. Multiple underscores are replaced with a single underscore after replacement.

Some basic examples:
- `Environment` to `EC2_TAG_ENVIRONMENT`
- `DatabaseName` to `EC2_TAG_DATABASENAME`
- `env--role` to `EC2_TAG_ENV_ROLE`
- `aws:autoscaling:groupName` to `EC2_TAG_AWS_AUTOSCALING_GROUPNAME`

Example Unit File
-----------------
The following unit file can be used to run the `ecr-env` tool on boot:

    [Unit]
    Description=Retrieve AWS EC2 environment
    After=network.target
    [Service]
    Type=oneshot
    RemainAfterExit=yes
    ExecStart=/opt/bin/ec2-env -file=/etc/environment.ec2
