FROM gcr.io/google-appengine/debian9:latest
RUN apt-get update && apt-get upgrade -y && apt-get install python -y
ADD docker-credential-gcr /bin/
ADD . /app
RUN /bin/docker-credential-gcr configure-docker
CMD ["python", "/app/main.py"]