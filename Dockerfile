# Stage 1: Build the application and dependencies
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make cmake gcc g++ libmad-dev \
    libid3tag-dev libsndfile-dev gd-dev boost-dev \
    libgd libpng-dev zlib-dev \
    zlib-static libpng-static boost-static libvorbis-static \
    autoconf automake libtool gettext samurai

# Build OGG
RUN git clone https://github.com/xiph/ogg && \
    cd ogg && \
    cmake -B build -G Ninja \
    -DCMAKE_INSTALL_PREFIX=/usr \
    -DCMAKE_INSTALL_LIBDIR=lib \
    -DBUILD_SHARED_LIBS=False \
    -DCMAKE_BUILD_TYPE=Release && \
    cmake --build build -j $(nproc) && \
    cmake --install build

# Build Opus
RUN git clone https://github.com/xiph/opus && \
    cd opus && \
    ./autogen.sh && \
    ./configure \
    --prefix=/usr \
    --sysconfdir=/etc \
    --localstatedir=/var \
    --enable-custom-modes \
    --enable-shared=no \
    --with-pic && \
    make -j $(nproc) && \
    make install

# Build FLAC
RUN git clone https://github.com/xiph/flac && \
    cd flac && \
    ./autogen.sh && \
    ./configure \
    --prefix=/usr \
    --enable-shared=no \
    --enable-ogg \
    --disable-rpath \
    --with-pic && \
    make -j $(nproc) && \
    make install

# Build libsndfile
RUN apk add --no-cache alsa-lib-dev flac-dev libvorbis-dev linux-headers python3 && \
    git clone https://github.com/libsndfile/libsndfile && \
    cd libsndfile && \
    cmake -B build -G Ninja \
    -DBUILD_SHARED_LIBS=OFF \
    -DCMAKE_BUILD_TYPE=MinSizeRel \
    -DCMAKE_INSTALL_PREFIX=/usr \
    -DENABLE_MPEG=ON && \
    cmake --build build -j $(nproc) && \
    cmake --install build

# Build libid3tag
RUN git clone https://codeberg.org/tenacityteam/libid3tag && \
    cd libid3tag && \
    cmake -B build -G Ninja \
    -DBUILD_SHARED_LIBS=OFF \
    -DCMAKE_BUILD_TYPE=MinSizeRel \
    -DCMAKE_INSTALL_PREFIX=/usr \
    -DCMAKE_INSTALL_LIBDIR=lib && \
    cmake --build build -j $(nproc) && \
    cmake --install build

# Build audiowaveform with static linking
RUN git clone https://github.com/bbc/audiowaveform.git /usr/src/audiowaveform && \
    cd /usr/src/audiowaveform && \
    mkdir build && \
    cd build && \
    cmake -DCMAKE_CXX_STANDARD=14 -D ENABLE_TESTS=0 -D BUILD_STATIC=1 .. && \
    make -j $(nproc) && \
    make install && \
    strip /usr/local/bin/audiowaveform


# Set up the Go application build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/handler .


# Stage 2: Create the final image
FROM alpine:3.22

# Install runtime dependencies
RUN apk add --no-cache libstdc++

# Copy the compiled Go application and audiowaveform binary
COPY --from=builder /usr/local/bin/audiowaveform /usr/local/bin/audiowaveform
COPY --from=builder /app/handler /app/handler

WORKDIR /app

# Set the entrypoint for the container
ENTRYPOINT ["/app/handler"]

