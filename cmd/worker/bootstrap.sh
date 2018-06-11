# this script bootstraps the rootfs of the container
# this should be installed at /root/bootstrap.sh
set -e

# symlink busybox
busybox --install -s /usr/bin

# unpack packages tar
mkdir /root/pkgs
tar -xf /root/pkgs.tar -C /root/pkgs

# unpack lpkg
tar -xf /root/pkgs/lpkg.tar.* -C /

# create database dir
mkdir -p /etc/lpkg.d/db

# install packages
for i in $(cat /root/pkgs/inst.list); do
    sh /usr/bin/lpkg-inst /root/pkgs/$i.tar.*
done
