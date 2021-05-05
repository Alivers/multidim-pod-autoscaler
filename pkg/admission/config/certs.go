// Package config  控制器访问api-server(https)需要的证书配置
package config

import (
	"io/ioutil"

	"k8s.io/klog"
)

type CertsContent struct {
	// 分别为 ca证书、服务器秘钥、服务器证书(由CA签发)
	CaCert, ServerKey, ServerCert []byte
}

type CertsConfig struct {
	// 证书及秘钥的路径
	ClientCaFile, TlsCertFile, TlsPrivateKey *string
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

func InitCerts(config CertsConfig) CertsContent {
	res := CertsContent{}
	res.CaCert = readFile(*config.ClientCaFile)
	res.ServerCert = readFile(*config.TlsCertFile)
	res.ServerKey = readFile(*config.TlsPrivateKey)
	return res
}
