FROM golang:1.21

WORKDIR /usr/src/pool_party/

COPY . .

RUN go build

CMD ./pool_party -listen 0.0.0.0:9080
