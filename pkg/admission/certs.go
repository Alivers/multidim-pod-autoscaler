// admission 控制器访问api-server(https)需要的证书配置
package main

import (
	"io/ioutil"

	"k8s.io/klog"
)

type certsContent struct {
	// 分别为 ca证书、服务器秘钥、服务器证书(由CA签发)
	caCert, serverKey, serverCert []byte
}

type certsConfig struct {
	// 证书及秘钥的路径
	clientCaFile, tlsCertFile, tlsPrivateKey *string
}

// readFile 读取 filePath 文件并将文件内容以 字节流返回
func readFile(filePath string) []byte {
	res, err := ioutil.ReadFile(filePath)
	if err != nil {
		klog.Errorf("Error reading certificate file at %s: %v", filePath, err)
		return nil
	}

	klog.V(3).Infof("Successfully read %d bytes from %v", len(res), filePath)
	return res
}

func initCerts(config certsConfig) certsContent {
	res := certsContent{}
	res.caCert = readFile(*config.clientCaFile)
	res.serverCert = readFile(*config.tlsCertFile)
	res.serverKey = readFile(*config.tlsPrivateKey)
	return res
}
