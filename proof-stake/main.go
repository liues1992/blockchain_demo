package proof_stake

import (
	"sync"
	"crypto/sha256"
	"encoding/hex"
	"time"
	"log"
	"io"
	"github.com/davecgh/go-spew/spew"
	"net"
	"bufio"
	"strconv"
	"fmt"
)

var Blockchain []Block	// 主区块链
var tempBlocks []Block	// 临时存放区块单元

// Block的通道，任何一个节点在提出一个新块时都将它发送到这个通道
var candidateBlocks = make(chan Block)

// 是一个通道，我们的主Go TCP服务器将向所有节点广播最新的区块链
var announcements = make(chan string)

// 控制读写和防数据竞争的标准变量
var mutex = &sync.Mutex{}

// 节点的存储map，同时也会保存每个节点持有的Token数
// key:地址 ; value:token 数
var validators = make(map[string]int)

// SHA256哈希
// calculateHash 是一个简单的计算SHA256的哈希函数
func calculateHash(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	hashed := h.Sum(nil)
	return hex.EncodeToString(hashed)
}

// calculateBlockHash 返回所有的block信息
func calculateBlockHash(block Block) string {
	record := string(block.Index) + block.Timestamp + string(block.BPM) + block.PrevHash
	return calculateHash(record)
}

// 创建新的块
func generateBlock(oldBlock Block, BPM int, address string) (Block, error) {
	var newBlock Block
	t := time.Now()
	newBlock.Index = oldBlock.Index + 1
	newBlock.Timestamp = t.String()
	newBlock.BPM = BPM
	newBlock.PrevHash = oldBlock.Hash
	newBlock.Hash = calculateBlockHash(newBlock)
	newBlock.Validator = address
	return newBlock, nil
}

// isBlockValid 是通过检查索引并比较前一个块的散列来确保块有效
func isBlockValid(newBlock, oldBlock Block) bool {
	if oldBlock.Index + 1 != newBlock.Index {
		return false
	}
	if oldBlock.Hash != newBlock.PrevHash {
		return false
	}
	if calculateBlockHash(newBlock) != newBlock.Hash {
		return false
	}
	return true
}

// 处理TCP的连接
func handleConn(conn net.Conn) {
	defer conn.Close()

	go func() {
		for {
			msg := <-announcements
			io.WriteString(conn, msg)
		}
	}()
	var address string	// 验证器的地址

	// 允许用户分配一定数量的权益tokena
	// tokens的数量越多, 生成新块的机会就越多
	io.WriteString(conn, "Enter token balance:")
	scanBalance := bufio.NewScanner(conn)
	for scanBalance.Scan() {
		balance, err := strconv.Atoi(scanBalance.Text())
		if err != nil {
			log.Printf("%v not a number: %v", scanBalance.Text(), err)
			return
		}
		t := time.Now()
		address = calculateHash(t.String())
		validators[address] = balance
		fmt.Println(validators)
		break
	}

	io.WriteString(conn, "\nEnter a new BPM:")

	scanBPM := bufio.NewScanner(conn)

	go func() {
		for {
			// 在进行必要的验证后，从stdin输入BPM 并将其添加到区块链
			for scanBPM.Scan() {
				bpm, err := strconv.Atoi(scanBPM.Text())

				// 如果验证者试图提议一个被污染（例如伪造）的block，例如包含一个不是整数的BPM，
				// 那么程序会抛出一个错误，我们会立即从我们的验证器列表validators中删除该验证者，
				// 他们将不再有资格参与到新块的铸造过程同时丢失相应的抵押令牌。
				if err != nil {
					log.Printf("%v not a number: %v", scanBPM.Text(), err)
					delete(validators, address)
					conn.Close()
				}

				mutex.Lock()
				oldLastIndex := Blockchain[len(Blockchain)-1]
				mutex.Unlock()

				// 我们用generateBlock函数创建一个新的block，
				// 然后将其发送到candidateBlocks通道进行进一步处理
				newBlock, err := generateBlock(oldLastIndex, bpm, address)
				if err != nil {
					log.Println(err)
					continue
				}
				if isBlockValid(newBlock, oldLastIndex) {
					candidateBlocks <- newBlock
				}
				io.WriteString(conn, "\nEnter a new BPM:")
			}
		}
	}()

	// 模拟接收广播
	for {
		time.Sleep(15*time.Second)

		io.WriteString(conn, "\n")
		mutex.Lock()
		spew.Fdump(conn, Blockchain)
		mutex.Unlock()
		io.WriteString(conn, "\n")
	}
}