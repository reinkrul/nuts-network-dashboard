# nuts-network-dashboard

This is a dashboard for the Nuts Network. It shows information about the network, such as the number of nodes and the number of transactions.

It gets its data from a Nuts node's diagnostics endpoint.

## Configuration

The dashboard can be configured using environment variables:

- `DASHBOARD_NODE_ADDR`: base URL of the Nuts node
- `DASHBOARD_TITLE`: title of the dashboard
- `DASHBOARD_DEBUG`: enable debug mode (set to `1` to enable)

## Running

The dashboard can be run using Docker:

```shell
docker run -p 8080:8080 \
  -e DASHBOARD_NODE_ADDR=http://nuts-node:1323 \
  -e DASHBOARD_TITLE="Nuts Network Dashboard" \
  reinkrul/nuts-network-dashboard:latest
```