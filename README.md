# Weir修改文档

> Weir原文档请看 [这里](README-EN.md)

> cj文件夹里的内容是我在实习期间的一些笔记以及测试文件，可以作为参考

> 我在截止到2021-08-31的Weir最新版本的基础上修复了一些问题并添加了一些新功能，具体内容如下

## 添加/修改内容介绍

* 参照 [Gaea-cc](https://github.com/XiaoMi/Gaea/blob/master/docs/gaea-cc.md) 的实现方式实现了weir-cc，目前有list、detail、modify、delete四个功能

* weir的admin端添加了ping接口

* 修改了configCenter为etcd时的代码逻辑（之前的有点混乱）

  > 讲下修改后的逻辑：
  >
  > 现在etcd的BasePath为"weir"，BasePath下有“proxy”和“namespace”两个分支，分别存储某集群中的所有weir节点信息和namespace租户信息
  >
  > 例如，我的"default"集群下只有一个weir节点，我将它命名为"weir1"，里面只有"test_namespace"这一个租户，那么它们在etcd中的存储路径就是 "[addr]/weir/proxy/default/weir1" 和 "[addr]/weir/namespace/default/test_namespace"
  >
  > 
  >
  > 另外，为了在etcd中存储namespace信息，我添加了namespace的json标签以及解析

* 实现了IP黑名单（测试weir的时候发现这个功能虽然在配置文件里，但是没有具体实现）

  > 模仿IsDatabaseAllowed函数写的，在driver和namespace文件夹下的各处添加了声明，最终选择在proxy/driver/queryctx.go+253的Auth()函数中进行调用

* 以及修复了一些bug，详细内容请见[Weir中发现的问题](cj/weir_problems.md)

