FROM golang:1.6.1
MAINTAINER damian@murf.org

COPY ./bin/ups-exporter /

EXPOSE 9099

ENTRYPOINT ["/ups-exporter"]
