FROM l.gcr.io/google/ubuntu16_04
RUN apt-get update && apt-get install -y curl
RUN echo "hey" > /etc/foo
RUN echo "baz" > /etc/foo
RUN echo "baz" > /etc/foo2

COPY . foo