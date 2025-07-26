# WordGate Mods - Configuration Guide

This directory contains reusable modules for the WordGate platform. Before using these modules, you need to configure the sensitive values marked with placeholders.

## Configuration Files

### AWS Configuration (`aws_config.yml` and `log/config.yml`)

Replace the following placeholders:
- `YOUR_AWS_ACCESS_KEY`: Your AWS access key ID
- `YOUR_AWS_SECRET_KEY`: Your AWS secret access key  
- `YOUR_REGION`: Your AWS region (e.g., "us-east-1", "ap-east-1")
- `YOUR_S3_BUCKET`: Your S3 bucket name
- `YOUR_ACCOUNT_ID`: Your AWS account ID
- `YOUR_QUEUE_NAME`: Your SQS queue name
- `YOUR_LOG_GROUP`: Your CloudWatch log group name

### Aliyun Configuration (`aliyun_config.yml`, `aliyun_ecs.yml`)

Replace the following placeholders:
- `YOUR_ALIYUN_ACCESS_KEY`: Your Aliyun access key ID
- `YOUR_ALIYUN_ACCESS_SECRET`: Your Aliyun access secret
- `YOUR_ALIYUN_LOG_ACCESS_KEY`: Your Aliyun log service access key
- `YOUR_ALIYUN_LOG_ACCESS_SECRET`: Your Aliyun log service access secret
- `YOUR_ALIYUN_ECS_ACCESS_KEY`: Your Aliyun ECS access key
- `YOUR_ALIYUN_ECS_SECRET`: Your Aliyun ECS secret
- `YOUR_PROJECT.YOUR_REGION.log.aliyuncs.com`: Your Aliyun log service endpoint
- `YOUR_LOGSTORE_NAME`: Your Aliyun log store name
- `YOUR_REGION_ID`: Your Aliyun region ID

### Email Configuration (`mail_config.yml`)

Replace the following placeholders:
- `YOUR_EMAIL@example.com`: Your email address
- `YOUR_EMAIL_PASSWORD`: Your email password or app password
- `YOUR_SMTP_HOST`: Your SMTP server host

### Third-party Service Configuration

#### GoDaddy (`godaddy.yml`)
- `YOUR_GODADDY_API_KEY`: Your GoDaddy API key
- `YOUR_GODADDY_API_SECRET`: Your GoDaddy API secret

#### Slack (`slack_config.yml`)
- `YOUR_SLACK_WEBHOOK_URL`: Your Slack webhook URL path

#### App Store (`appstore/appstore.yml`)
- `YOUR_APP_STORE_API_KEY_ID`: Your App Store Connect API key ID
- `YOUR_ISSUER_ID`: Your App Store Connect issuer ID
- `YOUR_PRIVATE_KEY_CONTENT_HERE`: Your App Store Connect private key content

#### Exchange Rate API (`exchange_rate_api_test.go`)
- `YOUR_EXCHANGE_RATE_API_KEY`: Your exchange rate API key

## Security Best Practices

1. **Never commit real credentials** to version control
2. **Use environment variables** for production deployments
3. **Store private keys securely** outside of configuration files when possible
4. **Rotate credentials regularly**
5. **Use least-privilege access** for all API keys and credentials
6. **Enable logging and monitoring** for credential usage

## Environment Variables

You can also configure these values using environment variables or a secure configuration management system instead of directly editing the YAML files.