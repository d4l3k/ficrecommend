FROM golang

ADD . /go/src/github.com/d4l3k/ficrecommend

RUN go get -v github.com/d4l3k/ficrecommend
RUN go install -v github.com/d4l3k/ficrecommend


EXPOSE 6060

WORKDIR /go/src/github.com/d4l3k/ficrecommend
VOLUME ["/var/ficrecommend"]
ENTRYPOINT ["/go/bin/ficrecommend", "--dbpath=/var/ficrecommend"]
