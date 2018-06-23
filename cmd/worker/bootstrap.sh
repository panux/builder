# this script bootstraps the rootfs of the container
# this should be installed at /root/bootstrap.sh
set -e

mkdir /hax

# symlink busybox so we can access it during the build
busybox --install -s /hax
export PATH=/hax:$PATH

# unpack packages tar
mkdir /root/pkgs
tar -xf /root/pkgs.tar -C /root/pkgs

# unpack lpkg
tar -xf /root/pkgs/lpkg.tar.* -C /

# create database dir
mkdir -p /etc/lpkg.d/db

INSTALLEDPKGS=

infoval() {
    (
        . "$1"
        local val="$2"
        eval "echo \$$val"
    ) || return $?
}

mkdir /tmp/inst

installpkg() {
    for i in $INSTALLEDPKGS; do
        # if it is already on the list, skip it
        if [ $i == $1 ]; then
            return
        fi
    done

    # add to installed list
    INSTALLEDPKGS="$INSTALLEDPKGS $1"

    # install dependencies first
    tar -xOf /root/pkgs/$1.tar.* ./.pkginfo > /tmp/inst/$1.pkginfo
    for i in $(infoval /tmp/inst/$1.pkginfo DEPENDENCIES); do
        installpkg $i
    done

    # install
    sh /usr/bin/lpkg-inst /root/pkgs/$1.tar.*
}

# install packages
for i in $(cat /root/pkgs/inst.list); do
    installpkg $i
done

rm -r /tmp/inst
