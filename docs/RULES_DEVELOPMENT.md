# Basic tips for developing new rules

- Positively identifying a service is generally preferred to attempting to detect something like "tenant not found"
- The [rules-development](../rules-development) directory contains rules that need improving
- If you don't have an account on a service the [web archive](https://web.archive.org/), [Security Trails](https://securitytrails.com/) and others have proved useful

## Testing a single rule

To run an A/B test against a single rule file specify the file with the `-r, --rules` flag (Testing via Burp suite):
```sh
./saasquatch --rules rules-development/canva.yml --proxy http://localhost:8080
```
You will then get information about which of the rules in the file are performing as desired:
```
INFO Starting SaaSquatch...
INFO Performing A/B test on existing rules...
WARN [-] Rule failed test Rule="Canva Public Profile"
INFO 'Test Rules' mode finished.
INFO Stats: Working rules=1 Total Rules=2 Percentage=0.500
INFO SaaSquatch run completed successfully.
```

## Please submit your rules

There are rule templates in [rules-development](../rules-development) to help you get started, we would love to see any rules you come up with to improve detection success rates.