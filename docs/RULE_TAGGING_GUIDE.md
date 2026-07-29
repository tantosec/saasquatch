### Tagging Guidelines

#### Identifier Input Type

Most rules treat the identifier as a **subdomain label** (`acme` → `acme.okta.com`) — this is the default and needs no tag. The exception is tagged explicitly, because it changes which wordlist you feed:

*   `full-domain`: The identifier is a full email/DNS domain, not a label (e.g. `acme.com` → `…?login=user@acme.com`). Run these with a domain wordlist, separate from subdomain rules: `--tags full-domain -I domains.txt`.

#### General Business & Productivity

*   `crm`: Customer Relationship Management (e.g., Salesforce, HubSpot)
*   `erp`: Enterprise Resource Planning (e.g., NetSuite, SAP)
*   `hr`: Human Resources & Payroll (e.g., Workday, BambooHR)
*   `finance`: Accounting & Payment Processing (e.g., QuickBooks, Stripe)
*   `project-management`: Task and Project Tracking (e.g., Jira, Asana, Trello)
*   `collaboration`: Team Chat & Document Sharing (e.g., Slack, Microsoft Teams, Confluence)
*   `communication`: Broader communication platforms (e.g., Zoom, Twilio)
*   `analytics`: Business Intelligence & Data Analytics (e.g., Tableau, Looker)
*   `marketing`: Marketing Automation & Tools (e.g., Mailchimp, Marketo)
*   `support`: Customer Support & Helpdesk (e.g., Zendesk, Intercom)

#### Development & Technical Operations

*   `dev`: General-purpose developer tools
*   `vcs`: Version Control Systems (e.g., GitHub, GitLab, Bitbucket)
*   `ci-cd`: Continuous Integration & Deployment (e.g., Jenkins, CircleCI, GitHub Actions)
*   `cloud`: Cloud Infrastructure & Platforms (e.g., AWS, GCP, Azure)
*   `monitoring`: Observability & Application Monitoring (e.g., Datadog, New Relic)
*   `idp`: Identity Provider & Single Sign-On (SSO) (e.g., Okta, Azure AD, Auth0)
*   `security`: Security Scanning & Compliance (e.g., Snyk, Qualys)
*   `storage`: Cloud Storage & File Sharing (e.g., Dropbox, Box)

#### Creative & Niche

*   `design`: Creative & Design Tools (e.g., Figma, Adobe Creative Cloud)
*   `social`: Social Media Platforms (e.g., Twitter, LinkedIn)
*   `e-commerce`: E-commerce Platforms (e.g., Shopify, Magento)