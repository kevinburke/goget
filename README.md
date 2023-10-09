# goget

Basically I like laying out my code directories the way that GOPATH did, but Go
has now moved away from this file layout. Make it easy to clone stuff into the
same directories that Go did.

### Usage

It should be a drop in replacement for the old "go get". Ensure you have
a GOPATH set.

At the moment it only uses SSH urls to do the clone.
