ROOT=$(cd $(dirname "${BASH_SOURCE}")/.. && pwd -P)
LOCAL_OUTPUT_ROOT=${ROOT}/_output

function build_image() {
  rm -rf ${LOCAL_OUTPUT_ROOT}
  mkdir -p ${LOCAL_OUTPUT_ROOT}
  GITBRANCH=$(git branch | grep \* | cut -d ' ' -f2)
  GITCOMMIT=$((git rev-parse -q HEAD)|cut -c1-12)
  cat > "${LOCAL_OUTPUT_ROOT}/Dockerfile" << EOF
FROM golang:1.15.4-alpine3.12 as binbuilder
ADD . /root/persistentevent
WORKDIR /root/persistentevent
RUN export GOPATH=/go && GO111MODULE=on GOOS=linux GOARCH=amd64 go build -o tke-event-watcher cmd/tke_event_watcher.go

FROM alpine:3.12
COPY --from=binbuilder /root/persistentevent/tke-event-watcher /root/persistentevent/tke-event-watcher
ENV GITBRANCH=${GITBRANCH}
ENV GITCOMMIT=${GITCOMMIT}
WORKDIR /root/persistentevent
CMD /root/persistentevent/tke-event-watcher
EOF

  docker build -f ${LOCAL_OUTPUT_ROOT}/Dockerfile -t tkestack/tke-event-watcher:v1.0-$((git rev-parse -q HEAD)|cut -c1-12) .
}

build_image
