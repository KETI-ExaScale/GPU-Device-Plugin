registry="ketidevit2"
imagename="keti-cluster-manager"
version="v0.23"

#gpu-scheduler binary file
go build -a --ldflags '-extldflags "-static"' -tags netgo -installsuffix netgo . && \

# make image
docker build -t $imagename:$version . && \

# add tag
docker tag $imagename:$version $registry/$imagename:$version && \ 

# login
docker login && \

# push image
docker push $registry/$imagename:$version 
