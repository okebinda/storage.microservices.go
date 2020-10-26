# -*- mode: ruby -*-
# vi: set ft=ruby :

Vagrant.configure(2) do |config|

  config.vm.box = "ubuntu/focal64"
  config.vm.synced_folder ".", "/vagrant", id: "vagrant-root", nfs: true, mount_options: ['actimeo=1']
  config.vm.provision :shell, path: "provision/development/bootstrap.sh"
  
  config.vm.hostname = "storage.microservices.go.vm"
  config.vm.network :private_network, ip: "172.29.5.100"
  
  config.vm.provider "virtualbox" do |vb|
    vb.name = "STORAGE-MICROSERVICES-GO-VM"
    vb.memory = 2048
  end

end
