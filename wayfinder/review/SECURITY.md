# Security Policy

## Supported Versions

We provide security updates for the following versions:

| Version | Supported          |
| ------- | ------------------ |
| 0.1.x   | :white_check_mark: |

## Reporting a Vulnerability

**DO NOT** open public GitHub issues for security vulnerabilities.

Instead, please report security vulnerabilities responsibly:

### Reporting Process

1. **Email**: Send details to security@wayfinder.dev
2. **Include**:
   - Description of the vulnerability
   - Steps to reproduce
   - Potential impact
   - Suggested fix (if you have one)
   - Your contact information for follow-up

### What to Expect

- **Acknowledgment**: Within 48 hours
- **Initial assessment**: Within 5 business days
- **Status updates**: Every 7 days until resolved
- **Resolution timeline**: Depends on severity
  - Critical: 7 days
  - High: 30 days
  - Medium: 60 days
  - Low: 90 days

### Coordinated Disclosure

- We will work with you to understand and validate the issue
- We will develop a fix and test it thoroughly
- We will prepare a security advisory
- We will coordinate a public disclosure timeline
- Credit will be given to reporters (unless anonymity is requested)

## Security Considerations

### API Keys

**CRITICAL**: Never commit API keys to source control

- Use environment variables for API keys:
  - `ANTHROPIC_API_KEY`
  - `GOOGLE_APPLICATION_CREDENTIALS` (for Vertex AI)
- Add `.env` to `.gitignore`
- Use secret management in CI/CD (GitHub Secrets, etc.)

### Code Review Output

- Review outputs may contain sensitive code snippets
- Use `--output-format json` and filter sensitive data before logging
- Be cautious when posting review results in public issue trackers

### Persona Prompts

- Custom personas may be executed with AI provider access
- Validate persona sources before use
- Avoid personas from untrusted sources
- Persona prompts have access to your code

### Dependency Security

- Dependencies are regularly scanned with Dependabot
- Security patches are applied promptly
- Run `npm audit` before installing/updating

### Best Practices

1. **Keep dependencies updated**:
   ```bash
   npm audit fix
   ```

2. **Use read-only API keys** when possible (provider-dependent)

3. **Limit code access**: Review only the code that needs review, not entire repositories with secrets

4. **Monitor costs**: Use cost tracking features to prevent unexpected charges

5. **Validate inputs**: The plugin validates inputs, but custom integrations should also validate

6. **Network security**:
   - Plugin communicates with AI providers over HTTPS
   - Ensure your network allows HTTPS egress

7. **Least privilege**: Run with minimal permissions needed

## Vulnerability Disclosure Policy

When we receive a security report:

1. We will confirm receipt within 48 hours
2. We will investigate and validate the issue
3. We will develop and test a fix
4. We will release a patched version
5. We will publish a security advisory
6. We will credit the reporter (if desired)

## Security Updates

Subscribe to security updates:

- Watch the repository for release notifications
- Enable Dependabot alerts
- Monitor the security advisories page

## Known Security Limitations

1. **Code exposure to AI providers**: Your code is sent to AI providers (Anthropic, Google). Ensure you trust these providers and review their privacy policies.

2. **Prompt injection**: Malicious code comments could potentially influence review outputs. Treat review outputs as suggestions, not trusted facts.

3. **Cost attacks**: Malicious PRs with large file changes could incur significant API costs. Implement cost limits and monitoring.

## Security Hardening Checklist

For production deployments:

- [ ] API keys stored in secure secret management
- [ ] Environment variables not logged
- [ ] Cost tracking and limits configured
- [ ] File size limits configured (if applicable)
- [ ] Review outputs sanitized before public display
- [ ] Dependencies up to date
- [ ] Security scanning enabled in CI/CD
- [ ] Incident response plan documented

## Contact

For security concerns:
- Email: security@wayfinder.dev
- PGP key: Available on request

For general questions:
- GitHub Discussions
- Standard issue tracker

---

Thank you for helping keep Multi-Persona Review secure!
