FROM prom/busybox:glibc

COPY ./ups-exporter /bin/ups-exporter

EXPOSE 9099

ENTRYPOINT ["/bin/ups-exporter"]
