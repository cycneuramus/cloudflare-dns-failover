# Cloudflare DNS Failover

A lightweight bash script that provides a simple yet effective solution for automatic DNS failover using Cloudflare's API.

The script monitors a list of server IPs in order of priority and updates the A record for a specified domain to the IP address of the first responsive server. This ensures high availability and reliability of services by automatically switching to a backup server in case the primary server goes down.

## Prerequisites

Before you begin, ensure you have met the following requirements:

- A Cloudflare account with access to API tokens.
- The `curl` and `jq` utilities installed on your system.

## Installation

1. **Clone the repository:**

```bash
git clone <repository-url>
cd <repository-folder>
```

2. **Set up your environment**

Open the script with your preferred text editor and edit the placeholder values:

- `CF_API_TOKEN`: Your Cloudflare API token with permissions to edit DNS records.
- `CF_ZONE_ID`: The Zone ID of the domain you are managing.
- `CF_RECORD`: The DNS record you wish to update.
- `SERVER_IPS`: A comma-separated list of server IPs, in order of priority.
- `CHECK_INTERVAL`: Interval (in seconds) between checks for server availability.

3. **Make the script executable**

```bash
chmod +x cloudflare-dns-failover.sh
```

## Usage

To start the DNS failover mechanism, simply run the script:

```bash
./cloudflare_dns_failover.sh
```

The script executes in an infinite loop, checking the availability of the servers at the specified interval and updating the DNS record as needed.

To maintain continuous operation, ensure that the script is running in a stable environment (e.g. as a `systemd` service or as an orchestrated `exec` job in a [Nomad](https://www.nomadproject.io/) cluster).