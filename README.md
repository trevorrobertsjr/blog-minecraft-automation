# blog-minecraft-automation

## Getting Started
First set the following Pulumi variables:

```bash
pulumi config set allowedCidrRanges '["your-ip-1/32","your-ip-2/32","your-ip-etc/32"]'
pulumi config set instanceType t4g.small
pulumi config set amiSsm /aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-arm64
pulumi config set tagKey app
pulumi config set tagValue minecraft-blog
pulumi config set route53HostName your-fqdn
pulumi config set route53ZoneId your-hosted-zone-id
pulumi config set vpcCidr 10.0.0.0/16
pulumi config set keypair your-keypair
```