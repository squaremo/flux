FROM alpine:3.6

LABEL org.label-schema.schema-version="1.0" \
      org.label-schema.name="flux" \
      org.label-schema.description="The Flux daemon, for synchronising your cluster with a git repo, and deploying new images" \
      org.label-schema.url="https://github.com/weaveworks/flux" \
      org.label-schema.vcs-url="git@github.com:weaveworks/flux" \
      org.label-schema.vendor="Weaveworks"

WORKDIR /home/flux
ENTRYPOINT [ "/sbin/tini", "--", "fluxd" ]
RUN apk add --no-cache openssh ca-certificates tini 'git>=2.3.0'

# Add git hosts to known hosts file so when git ssh's using the deploy
# key we don't get an unknown host warning.
RUN mkdir ~/.ssh && touch ~/.ssh/known_hosts && \
    ssh-keyscan github.com gitlab.com bitbucket.org >> ~/.ssh/known_hosts && \
    chmod 600 ~/.ssh/known_hosts

COPY ./kubectl /usr/local/bin/
COPY ./fluxd /usr/local/bin/
