# Cloudflare DNS Failover

A small Go program that provides a simple yet effective solution for automatic DNS failover using [Cloudflare's](https://www.cloudflare.com/application-services/products/dns) API.

The program monitors a list of server IPs in order of priority and updates the A record for a specified domain to the IP address of the first responsive server. This ensures high availability and reliability of services by automatically switching to a backup server in case the primary server goes down.

This DNS failover mechanism is not to be confused with [round-robin DNS](https://en.wikipedia.org/wiki/Round-robin_DNS), where multiple A records are set for the same domain to distribute traffic randomly across different servers. While potentially useful, this cannot be depended upon for site reliability, since if one of the servers goes down, the DNS server will still keep that server’s IP in the round-robin rotation.

## Prerequisites

Before you begin, ensure you have met the following requirements:

- A Cloudflare account with access to API tokens.

## Installation

1. **Clone the repository:**

```bash
git clone https://github.com/cycneuramus/cloudflare-dns-failover
cd cloudflare-dns-failover
```

2. **Configure your records**

Open the `config.yml.example` with your preferred text editor and edit the placeholder values. Save the file as `config.yml`.

3. **Build the program**

```bash
go build .
```

## Usage

To start the DNS failover mechanism, simply run the program:

```bash
./cloudflare-dns-failover
```

This assumes that `config.yml` exists in the working directory. You can, however, pass its path as a flag:

```bash
./cloudflare-dns-failover -c /path/to/config.yml
```

The program executes in an infinite loop, checking the availability of the servers at the specified interval and updating the DNS record as needed.

To maintain continuous operation, ensure that the program is running in a stable environment (e.g. as a `systemd` service or perhaps even as an orchestrated `exec` job in a [Nomad](https://www.nomadproject.io/) cluster).
