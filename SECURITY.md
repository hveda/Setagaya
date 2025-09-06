# Security Policy

## Supported Versions

We actively support security updates for the following versions of Setagaya:

| Version | Supported          |
| ------- | ------------------ |
| 2025.x  | :white_check_mark: |
| 2024.x  | :x:                |
| < 2024  | :x:                |

## Reporting a Vulnerability

We take security vulnerabilities seriously. If you discover a security vulnerability within Setagaya, please follow
these steps:

### 🔒 Private Disclosure

**DO NOT** create a public GitHub issue for security vulnerabilities.

Instead, please report security vulnerabilities through:

1. **GitHub Security Advisories**: Use the "Security" tab in this repository
2. **Email**: Send to security@yourorganization.com (replace with actual contact)
3. **Private Issue**: Contact maintainers directly via GitHub

### 📝 What to Include

When reporting a vulnerability, please include:

- **Description**: Clear description of the vulnerability
- **Impact**: Potential impact and attack scenarios
- **Steps to Reproduce**: Detailed steps to reproduce the issue
- **Proof of Concept**: Code or screenshots demonstrating the vulnerability
- **Affected Versions**: Which versions are affected
- **Suggested Fix**: If you have ideas for a fix

### ⏱️ Response Timeline

We aim to respond to security reports according to the following timeline:

- **Initial Response**: Within 24 hours
- **Vulnerability Assessment**: Within 72 hours
- **Status Update**: Weekly updates until resolution
- **Fix Release**: Critical issues within 7 days, others within 30 days

### 🏆 Recognition

We believe in recognizing security researchers who help us maintain a secure platform:

- **Hall of Fame**: Public recognition (with permission)
- **Acknowledgment**: Credit in release notes
- **Coordination**: We'll work with you on responsible disclosure timing

## Security Measures

### 🔐 Built-in Security Features

Setagaya includes several security measures:

- **LDAP Authentication**: Enterprise authentication integration
- **RBAC**: Role-based access control for projects and collections
- **Secure Containers**: All containers run as non-root users
- **Network Policies**: Kubernetes network isolation
- **Secret Management**: Secure handling of sensitive data
- **Audit Logging**: Comprehensive activity logging

### 🛡️ Deployment Security

For secure deployments:

- **TLS Encryption**: Always use HTTPS in production
- **Network Isolation**: Deploy in private subnets
- **Resource Limits**: Configure appropriate resource constraints
- **Regular Updates**: Keep dependencies and base images updated
- **Monitoring**: Enable security monitoring and alerting

### 🔍 Security Scanning

We perform regular security scanning:

- **Dependency Scanning**: Automated vulnerability scanning of Go modules
- **Container Scanning**: Security scanning of Docker images
- **Static Analysis**: Code analysis for security issues
- **Secret Scanning**: Detection of exposed secrets
- **License Compliance**: Open source license verification

## Security Best Practices

### For Developers

- **Secure Coding**: Follow secure coding practices
- **Dependencies**: Keep dependencies updated
- **Secrets**: Never commit secrets to the repository
- **Testing**: Include security testing in your workflows
- **Code Review**: Mandatory security-focused code reviews

### For Operators

- **Access Control**: Implement least-privilege access
- **Network Security**: Use firewalls and network segmentation
- **Monitoring**: Enable comprehensive logging and monitoring
- **Backup Security**: Secure backup storage and encryption
- **Incident Response**: Have an incident response plan

### For Users

- **Authentication**: Use strong, unique passwords
- **Access**: Only grant necessary permissions
- **Monitoring**: Monitor for unusual activity
- **Updates**: Keep your Setagaya installation updated
- **Reporting**: Report suspicious activity immediately

## Vulnerability Disclosure Policy

### Our Commitment

- We will acknowledge receipt of your vulnerability report
- We will provide regular updates on our progress
- We will credit you for your discovery (with your permission)
- We will not take legal action against good-faith security research

### Scope

This policy applies to:

- ✅ Setagaya core application and components
- ✅ Official Docker images and containers
- ✅ Documentation and example configurations
- ✅ Dependencies and third-party components

This policy does not apply to:

- ❌ Issues in third-party systems not controlled by us
- ❌ Social engineering attacks
- ❌ Physical attacks
- ❌ Denial of service attacks

### Guidelines for Researchers

When testing for vulnerabilities:

- **Respect Privacy**: Don't access user data
- **Minimize Impact**: Don't disrupt service availability
- **Responsible Testing**: Use test environments when possible
- **Legal Compliance**: Follow applicable laws and regulations
- **Coordinated Disclosure**: Work with us on disclosure timing

## Security Contact

For security-related questions or concerns:

- **Security Team**: security@yourorganization.com
- **GitHub Security**: Use the Security tab in this repository
- **PGP Key**: Available upon request

---

_This security policy is reviewed and updated quarterly. Last updated: September 2025_
