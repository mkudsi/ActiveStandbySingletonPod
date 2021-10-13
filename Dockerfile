FROM  ubuntu:18.04
MAINTAINER  Mansoorali Kudsi
RUN apt-get update
RUN apt-get install net-tools
RUN apt-get install -y iputils-ping
COPY icr /usr/bin/
ENTRYPOINT ["/usr/bin/icr"]

