# -*- mode: ruby -*-
# # vi: set ft=ruby :

unless Vagrant.has_plugin?("vagrant-libvirt")
  raise Vagrant::Errors::VagrantError.new, "Please install the vagrant-libvirt plugin running 'vagrant plugin install vagrant-libvirt'"
end

Vagrant.configure("2") do |config|
  config.vm.define :pxeserver do |pxeserver|
    pxeserver.vm.box = "naelyn/ubuntu-trusty64-libvirt"
    pxeserver.vm.network :private_network, :ip => '10.10.10.2'
    pxeserver.vm.provision :shell, path: 'vagrant_provision.sh'
  end

  config.vm.define :pxeclient1 do |pxeclient1|
    pxeclient1.vm.provider :libvirt do |pxeclient1_vm|
      pxeclient1_vm.storage :file, :size => '20G', :type => 'qcow2'
      pxeclient1_vm.boot 'network'
      pxeclient1_vm.boot 'hd'
    end
  end
end

