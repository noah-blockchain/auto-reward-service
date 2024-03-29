# docker build -t noah-extender:latest -f DOCKER/Dockerfile .
# docker build --no-cache -t node:latest -f DOCKER/Dockerfile .
# docker run -d -p 127.0.0.1:9000:9000 --restart=always noah-extender:latest

FROM golang:1.12-buster as builder

ENV APP_PATH /home/auto_rewards

COPY . ${APP_PATH}

WORKDIR ${APP_PATH}

RUN make create_vendor && make build

FROM debian:buster-slim as executor
COPY --from=builder /home/auto_rewards/builds/auto_rewards /usr/local/bin/auto_rewards
CMD ["auto_rewards"]
STOPSIGNAL SIGTERM
