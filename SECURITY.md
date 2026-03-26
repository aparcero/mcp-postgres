# Security Policy

## Supported Versions

The main branch is the supported development line until formal releases begin.

## Reporting a Vulnerability

Please do not report vulnerabilities in public issues.

Use GitHub Security Advisories if they are enabled for the repository. If private advisories are not available, open a minimal public issue asking for a private security contact and do not include exploit details.

Include enough information to reproduce the issue privately:

- Affected version or commit
- PostgreSQL version, server mode, configuration, and deployment environment
- MCP client or transport details
- Reproduction steps
- Expected and actual behavior
- Impact assessment

## Scope

Security-sensitive areas include:

- SQL parsing, classification, and policy enforcement
- Database credentials and connection-string handling
- Mutation and admin operation controls
- Confirmation-token behavior
- MCP tool argument handling
- Logging of SQL, errors, connection details, or query output
- Dependency updates

## Operational Guidance

Run this server with the least-privileged PostgreSQL role that supports the tools you enable. Parser-backed SQL checks and read-only transactions are defense-in-depth; they are not a substitute for database permissions, row-level security, schema privileges, network controls, backups, and audit logging.
