# pool_party

   Pool Party          |  Pool Party
:-------------------------:|:-------------------------:
![](https://i.imgur.com/ty2p7hZ.gif)  |  ![](https://media.giphy.com/media/41x8Gui7T1hEsZ0vSF/giphy-downsized-large.gif)

## Building
```
sudo apt install golang && go build
```

## Usage
```
Usage of pool_party:
  -listen string
        The IP and port to listen on (default "127.0.0.1:9080")
  -target string
        The target host and port to connect to (default "mainnet-pociot.helium.io:9080")
```
## docker-compose.yml example
```
---
version: "3.8"
services:
  
  miner:
    image: quay.io/team-helium/miner:gateway-latest
    links:
      - poolparty:mainnet-pociot.helium.io
    environment:
      - GW_REGION=US915
      - GW_KEYPAIR=ecc://i2c-1
      - GW_LISTEN=0.0.0.0:1680
      - GW_API=0.0.0.0:4467
    expose:
      - "1680/udp"
      - "4467/tcp"
    depends_on:
      - poolparty
    cap_add:
      - SYS_RAWIO
    devices:
      - /dev/i2c-1:/dev/i2c-1
    restart: unless-stopped

  poolparty:
    build:
      context: https://github.com/joecryptotoo/pool_party.git#main
    expose:
      - 9080/tcp
    restart: unless-stopped
```
