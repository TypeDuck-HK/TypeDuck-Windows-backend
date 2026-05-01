# 全拼候选 Reranker 训练流程

本文记录全拼输入候选重排模型的基本思路、数据格式、训练流程和测试集格式。目标不是让模型生成汉字，而是在 Rime 或其他输入法已经给出候选后，判断哪个候选更应该排在前面。

## 背景结论

现代全拼输入的主要难点不是“句子是否通顺”，而是：

- 同一串拼音可能对应多个合理汉字句子。
- 错误候选本身也可能很通顺，甚至比正确句更常见。
- 单纯语言模型容易偏向高频表达，而不是偏向当前拼音真正对应的句子。

例如：

```text
input: zhibuguoshiyigekanke
正确: 只不过是一个看客
错误: 只不过是一个坎坷
```

如果只看汉字句子，“坎坷”可能更常见、更自然；但结合拼音输入，正确候选应该是“看客”。

因此更合适的任务是 reranking：

```text
score(input, 正确候选) > score(input, 错误候选)
```

## 模型定位

推荐使用小型 Transformer Encoder 作为候选打分器，例如：

```text
底座: uer/chinese_roberta_L-2_H-128
层数: 2
hidden size: 128
attention heads: 2
参数量: 约 318 万
```

它不是直接作为 MLM 通顺度模型使用，而是加一个 ranking head：

```text
[CLS] input拼音 [SEP] 候选汉字句 [SEP]
        |
  小 Transformer Encoder
        |
  CLS 向量
        |
  Linear 层
        |
      score
```

候选列表里每个候选各跑一次打分，然后按 `score` 从高到低重排。

## 为什么不能直接用 MLM 打分

我们用 `uer/chinese_roberta_L-2_H-128` 对 benchmark 中的正确句和错误句做过 MLM pseudo-likelihood 测试：

```text
全部错误 pair:
去重句子对: 754
gold 分数更高: 447
准确率: 59.28%

只看 rime_frost 错误:
错误句子对: 127
gold 分数更高: 58
准确率: 45.67%
```

这说明原始小 RoBERTa 能发现明显病句，但不能稳定修正输入法错误。原因是它只看汉字文本，不知道用户输入的拼音是什么。

Reranker 微调后，模型学习的是：

```text
在这串拼音 input 下，哪个候选更好
```

而不是泛泛判断：

```text
哪个汉字句子更常见、更通顺
```

## 训练集格式

推荐使用 JSONL，一行一个样本。

最小格式：

```jsonl
{"input":"tazhemeyihan","positive":"他这么一喊","negative":"他这么遗憾"}
{"input":"zhibuguoshiyigekanke","positive":"只不过是一个看客","negative":"只不过是一个坎坷"}
{"input":"yonghupaoguangle","positive":"用户跑光了","negative":"用户抛光了"}
```

字段含义：

```text
input: 用户输入的拼音串，通常是不带空格的全拼
positive: 正确候选，也就是 gold
negative: 错误候选，也就是 prediction 或其他错误候选
```

如果一个输入有多个错误候选，可以使用数组格式：

```jsonl
{"input":"tazhemeyihan","positive":"他这么一喊","negatives":["他这么遗憾","他这么一韩","他怎么一喊"]}
```

训练时展开为多条 pair：

```text
他这么一喊 > 他这么遗憾
他这么一喊 > 他这么一韩
他这么一喊 > 他怎么一喊
```

也可以保留来源字段，便于后续分析：

```jsonl
{"vendor":"rime_frost","corpus":"news","index":123,"input":"tazhemeyihan","positive":"他这么一喊","negative":"他这么遗憾","cer":0.25}
```

训练时真正必需的字段只有：

```text
input
positive
negative
```

## 从 benchmark CSV 构造训练数据

现有 benchmark CSV 字段类似：

```csv
corpus,index,vendor,gold,input,prediction,exact,cer,error
```

可以按下面规则转换：

```text
input      -> input
gold       -> positive
prediction -> negative
```

只取错误样本：

```text
exact == False
gold != prediction
```

如果只训练某个方案，例如 `rime_frost`：

```text
vendor == rime_frost
exact == False
gold != prediction
```

注意：当前单个 CSV 里 `rime_frost` 错误样本只有约 127 条，适合做验证集或测试集，不足以单独训练出稳定模型。训练集应尽量扩大到几千到几万 pair。

## 测试集格式

测试集也建议使用 JSONL，格式和训练集一致：

```jsonl
{"input":"tazhemeyihan","positive":"他这么一喊","negative":"他这么遗憾","vendor":"rime_frost","corpus":"news","index":1}
{"input":"zhibuguoshiyigekanke","positive":"只不过是一个看客","negative":"只不过是一个坎坷","vendor":"rime_frost","corpus":"news","index":2}
```

测试时计算：

```text
score_pos = model(input, positive)
score_neg = model(input, negative)
```

如果：

```text
score_pos > score_neg
```

则该样本判断正确。

核心指标：

```text
pairwise_accuracy = 判断正确的 pair 数 / 总 pair 数
```

可以额外按来源统计：

```text
by_vendor_accuracy
by_corpus_accuracy
by_cer_bucket_accuracy
```

如果有完整候选列表，测试集可以扩展为 listwise 格式：

```jsonl
{"input":"zhibuguoshiyigekanke","gold":"只不过是一个看客","candidates":["只不过是一个坎坷","只不过是一个看客","只不过是一个看可"],"vendor":"rime_frost"}
```

对应指标：

```text
top1_accuracy: 重排后第 1 个候选是否等于 gold
mrr: gold 在重排列表中的倒数排名
topk_recall: gold 是否出现在 Top K
```

当前推荐的回归质量分格式如下：

```jsonl
{"input":"gegeguojiadeguoge","input_syllables":"ge ge guo jia de guo ge","gold":"各个国家的国歌","candidates":[{"rank":1,"text":"各个国家德国个","quality":0.7142857},{"rank":2,"text":"各个国家的国歌","quality":1.0}]}
```

字段含义：

```text
input: 连续拼音
input_syllables: 带空格拼音切分
gold: 正确句子，仅训练/评估使用
candidates: 真实 Rime 候选列表
rank: Rime 原始候选序号
text: 候选文本
quality: 1 - edit_distance(candidate, gold) / max(len(candidate), len(gold))
```

## 负样本来源

训练效果主要取决于负样本质量。建议混合以下来源：

1. 真实输入法错误

来自 benchmark 的 `prediction`，最贴近实际问题。

```text
positive = gold
negative = prediction
```

2. 同音或近音替换

自动构造类似错误：

```text
看客 -> 坎坷
一喊 -> 遗憾
跑光 -> 抛光
情意 -> 轻易
```

3. 其他输入法候选

同一个 `input` 下，不同 vendor 的输出可以互相构造候选池。

4. Rime 候选列表

如果可以从 Rime 解码阶段拿到 Top N 候选，训练效果会更贴近最终使用场景。

## 数据量建议

最小实验：

```text
训练集: 1,000 到 5,000 pair
验证集: 200 到 1,000 pair
测试集: 固定保留真实 benchmark 错误
```

较稳妥：

```text
训练集: 10,000 到 50,000 pair
验证集: 2,000 到 5,000 pair
测试集: 1,000 到 5,000 pair
```

不要把最终要汇报的 benchmark 测试集混入训练集，否则评估结果会虚高。

## 训练目标

### 当前推荐：回归式质量分

经过多轮实验后，当前更推荐把 reranker 训练成“候选质量分回归模型”，而不是只做 pairwise 或 listwise 分类。

模型输入仍然是：

```text
score = model(input_pinyin, candidate_text, rank)
```

但 `score` 的含义改为 `0.0 ~ 1.0` 的候选质量分。推理时对同一个候选列表内每个候选分别打分，最终取分数最高的候选：

```text
best_candidate = argmax(model_score + rank_bias)
```

训练目标使用文字正确率归一化：

```text
quality = 1 - edit_distance(candidate, gold) / max(len(candidate), len(gold))
```

例如：

```text
gold:      各个国家的国歌
candidate: 各个国家德国个
```

按逐字 Levenshtein 计算，正确相当于 `5/7`，所以：

```text
quality = 5 / 7 = 0.7142857
```

注意：`quality` 只用于训练和评估。真实输入时没有 `gold`，所以推理阶段不能把 `quality` 作为输入特征。

推荐 loss：

```text
loss = SmoothL1Loss(pred_score, quality)
```

这种训练方式的优点是：

- gold 候选自然得到 `1.0`。
- 局部正确的候选能得到中间分数，而不是被粗暴视为 `0`。
- 模型学习的是“候选句和目标句有多接近”，更适合输入法候选重排。

### 历史方案：pairwise ranking

对每条样本构造两个输入：

```text
pos_text = input + [SEP] + positive
neg_text = input + [SEP] + negative
```

模型输出：

```text
score_pos = model(pos_text)
score_neg = model(neg_text)
```

推荐使用 margin ranking loss：

```text
loss = max(0, margin - score_pos + score_neg)
```

常用 margin：

```text
0.1 到 0.5
```

也可以使用二分类交叉熵，把 pair 展开成单条候选样本：

```jsonl
{"input":"tazhemeyihan","candidate":"他这么一喊","label":1}
{"input":"tazhemeyihan","candidate":"他这么遗憾","label":0}
```

但 pairwise loss 更直接对应“候选 A 应排在候选 B 前面”。

pairwise / listwise 分类仍然可以作为辅助目标，但当前主线建议先使用回归质量分验证端到端效果。

## 推荐输入格式

建议使用标准 BERT pair 输入，而不是手动把 `[SEP]` 当普通字符串拼进去：

```python
tokenizer(
    input_pinyin,
    candidate_text,
    truncation=True,
    max_length=128,
    padding=True,
    return_tensors="pt",
)
```

等价 token 结构：

```text
[CLS] input_pinyin [SEP] candidate_text [SEP]
```

这样 token type ids 可以区分拼音段和候选句段。

## 训练流程

1. 准备数据

从 benchmark CSV、Rime 候选、自动同音替换中构造 JSONL。

2. 切分数据

推荐按句子或 `index` 切分，而不是随机按行切分，避免同一个 gold 同时出现在训练集和测试集。

```text
train: 80%
valid: 10%
test: 10%
```

如果已有固定 benchmark，建议：

```text
train: 自动构造数据 + 非目标 benchmark
valid: 部分真实错误
test: 固定 rime_frost 错误样本
```

3. 加载底座模型

```text
uer/chinese_roberta_L-2_H-128
```

4. 添加 ranking head

```text
CLS hidden -> dropout -> linear -> score
```

5. 使用 pairwise loss 训练

每个 batch 同时计算 positive 和 negative 的 score。

6. 验证

每轮训练后计算 valid pairwise accuracy。

7. 测试

在固定 test JSONL 上输出整体准确率和错误案例。

8. 导出

先导出 PyTorch checkpoint；确认有效后，再考虑 ONNX / OpenVINO / INT8。

## 推理流程

输入法侧流程：

```text
1. 用户输入拼音
2. Rime 生成 Top N 候选
3. 对每个候选计算 score(input, candidate)
4. 融合原始候选分和 reranker 分
5. 重排候选栏
```

融合公式可以先用简单线性加权：

```text
final_score = original_score + alpha * reranker_score
```

如果拿不到 Rime 原始分，可以先只用 reranker 在 Top N 内重排。

## 耗时基准

在当前桌面 CPU 环境中，`uer/chinese_roberta_L-2_H-128` 的粗略耗时如下：

```text
PyTorch: 2.10.0+cpu
CUDA: False
CPU threads: 16
```

不含 tokenizer：

```text
单候选前向: 约 0.62 到 0.70 ms
4 候选 batch: 约 0.80 ms
8 候选 batch: 约 0.97 ms
```

包含 tokenizer 和模型前向：

```text
1 候选: 约 0.85 ms
4 候选 batch: 约 1.57 ms
8 候选 batch: 约 1.82 ms
```

这个耗时是 Python + PyTorch 下的结果。实际部署到 ONNX / OpenVINO / INT8 后通常还有优化空间。

注意：MLM pseudo-likelihood 评估需要对每个 token 分别 mask，不适合实时输入。真正 reranker 是“一条候选一次前向”。

## 第一阶段实验建议

第一阶段不要直接做复杂部署，先验证训练是否有效：

```text
1. 用 benchmark CSV 转 JSONL。
2. 固定保留 rime_frost 错误样本为 test。
3. 用其他 vendor 错误 + 自动同音替换构造 train。
4. 微调 2 层 RoBERTa reranker。
5. 对比训练前后 pairwise accuracy。
```

目标不是一次做到完美，而是确认：

```text
训练后 score(input, gold) > score(input, prediction) 的比例显著高于原始 MLM 打分。
```

如果 `rime_frost` 测试集能从 45.67% 提升到 70% 以上，就说明这个方向值得继续。

## 后续改进方向

- 增加真实 Rime Top N 候选作为 listwise 训练数据。
- 加入拼音切分信息，例如 `zhi bu guo shi yi ge kan ke`。
- 加入候选原始排序、词频、候选长度等特征。
- 使用 hard negative：错误但很通顺、很容易骗过语言模型的候选。
- 导出 ONNX 后评估端侧延迟。
- 尝试 INT8 量化，观察准确率和耗时变化。

## 2026-04 实验结论更新

### 正确评估环境

后续实验必须固定下面环境，否则 TopN 召回结论会明显不同：

```text
方案: rime_frost
DLL: d:\vscode\rime_projs\rime-schema-compare\lib\rime-24986039806.dll
配置: vendor\rime-frost\rime_frost.custom.yaml
```

当前 `rime_frost.custom.yaml`：

```yaml
patch:
  translator/enable_user_dict: false
  translator/max_sentences: 10
  translator/max_homophones: 10
```

之前如果没有显式指定 `rime-24986039806.dll`，会默认使用 `lib\rime.dll`，导致 Top3/Top10 候选召回结果不一致。

### 原始 Top3 召回

使用正确 DLL 后，原始 Top3 的上限明显高于 Top1：

```text
小语料 255 条:
Top1: 128/255 = 50.20%
Top3 gold in candidates: 167/255 = 65.49%

heldout 50001-70000:
Top1: 61.965%
Top3 gold in candidates: 75.90%

heldout 250001-270000:
Top1: 64.85%
Top3 gold in candidates: 79.12%
```

这说明真实 Rime Top3 里有可救空间，但模型必须学会在真实候选分布里稳定选择正确候选。

### 人工插入 gold 实验

为了判断 reranker 是否有能力在候选中选出正确句，做过人工召回增强实验：

```text
Top1 正确: 保持原始候选
Top1 错误: 构造 [原 Top1, gold, 原 Top2]
```

即 gold 放在 rank2，不放第一，也不放最后。

强监督模型在该设置下：

```text
50001-70000:
原始 Top1: 61.96%
Top3 + 模型: 85.28%
net delta: +4664

250001-270000:
原始 Top1: 64.86%
Top3 + 模型: 85.685%
net delta: +4165

合并 40000 条:
原始 Top1: 63.41%
Top3 + 模型: 85.4825%
net delta: +8829
```

结论：只要正确答案进入候选列表，reranker 有能力把大量 gold 提上来。

### 原始 Top3 + 旧强监督模型

用正确 DLL 跑原始 Top3 后，旧强监督模型仍未转正：

```text
50001-70000:
Top1: 61.965%
Top3 上限: 75.90%
纯模型: 33.095%
rank bias 后: 61.825%

250001-270000:
Top1: 64.85%
Top3 上限: 79.12%
纯模型: 34.905%
rank bias 后: 64.67%
```

原因是旧训练数据大量来自人工 append gold 或不匹配的候选分布，模型没有真正学会“真实 Rime Top3/Top10 候选”里的排序。

### 回归质量分数据

因此改为使用真实 Rime Top10 候选生成回归质量分数据。生成位置：

```text
E:\训练bert\data
```

数据文件：

```text
quality_rime_frost_top10_50k_var2_5.train.jsonl
quality_rime_frost_top10_50k_var2_5.valid.jsonl
quality_rime_frost_top10_50k_var2_5.test.jsonl
quality_rime_frost_top10_50k_var2_5.all.jsonl
quality_rime_frost_top10_50k_var2_5.summary.json
```

生成设置：

```text
source: zhihu_deal0.split_hanzi.txt
skip: 70000
limit: 50000
top_n: 10
每条样本候选数: 随机 2/3/4/5 个
quality: 1 - edit_distance(candidate, gold) / max(len(candidate), len(gold))
```

数据统计：

```text
总样本: 50000
train: 40000
valid: 5000
test: 5000
候选总数: 175303

候选数分布:
2 个: 12348
3 个: 12535
4 个: 12583
5 个: 12534

gold_rank:
0: 8844
1: 29970
2: 4936
3: 2094
4: 1292
5: 819
6-10: 2045

quality 平均值: 0.764
quality = 1.0: 36657
quality >= 0.8: 112104
quality >= 0.5: 149929
```

### 回归模型 1 epoch 结果

新增训练脚本：

```text
d:\vscode\rime_projs\rime-schema-compare\scripts\train_quality_regression_reranker.py
```

模型输出：

```text
E:\训练bert\models\quality_regression_l2_1epoch
```

训练配置：

```text
底座: uer/chinese_roberta_L-2_H-128
loss: SmoothL1Loss(pred_score, quality)
epoch: 1
batch_size: 64
GPU: RTX 4060
```

结果：

```text
initial valid:
MAE: 0.2729
pick_best_accuracy: 24.36%

1 epoch valid:
MAE: 0.1053
RMSE: 0.1481
pick_best_accuracy: 64.58%

1 epoch test:
MAE: 0.1047
RMSE: 0.1473
pick_best_accuracy: 66.34%
```

其中 `pick_best_accuracy` 表示模型选出的最高分候选是否等于真实 `quality` 最高的候选。

注意：测试集中原始 rank1 本来就是最高 quality 的比例是 `73.56%`，所以 1 epoch 回归模型还没有超过直接选 Top1，但已经显著学到质量分：MAE 从 `0.2729` 降到 `0.1047`。

### 下一步

后续优先方向：

1. 继续训练回归模型多轮，观察是否超过原始 rank1 的 `pick_best_accuracy`。
2. 加入排序辅助 loss，让模型不仅拟合 quality，还更重视组内最高 quality 的候选。
3. 对真实 Top3/Top10 评估 `model_score + rank_bias`，寻找能转正的融合参数。
4. 端侧部署前再做 ONNX / INT8 延迟评估。

## 当前实验路径清单

本节记录当前实验实际使用过的脚本、语料、模型和产物路径，便于复现。

### 仓库路径

```text
主输入法仓库:
d:\vscode\moqi-input-method-projs\moqi-ime

评测/训练脚本仓库:
d:\vscode\rime_projs\rime-schema-compare

Rime Frost 方案目录:
d:\vscode\rime_projs\rime-schema-compare\vendor\rime-frost
```

### Rime 配置和 DLL

必须显式使用下面 DLL，否则候选召回结果可能不同：

```text
d:\vscode\rime_projs\rime-schema-compare\lib\rime-24986039806.dll
```

当前使用的 Frost 自定义配置：

```text
d:\vscode\rime_projs\rime-schema-compare\vendor\rime-frost\rime_frost.custom.yaml
```

当前内容：

```yaml
patch:
  translator/enable_user_dict: false
  translator/max_sentences: 10
  translator/max_homophones: 10
```

### 原始语料

原始知乎语料：

```text
d:\vscode\rime-frost\cn_dicts_dazhu\zhihu_deal0.txt
```

清洗后纯汉字分句语料：

```text
d:\vscode\rime-frost\cn_dicts_dazhu\zhihu_deal0.split_hanzi.txt
```

用于 held-out 评估的切片语料：

```text
E:\训练bert\data\corpus\zhihu_deal0.split_hanzi.holdout_50001_70000.txt
E:\训练bert\data\corpus\zhihu_deal0.split_hanzi.holdout_250001_270000.txt
```

小规模固定评测语料：

```text
d:\vscode\rime_projs\rime-schema-compare\data\corpus\news.txt
d:\vscode\rime_projs\rime-schema-compare\data\corpus\novel.txt
d:\vscode\rime_projs\rime-schema-compare\data\corpus\prose.txt
d:\vscode\rime_projs\rime-schema-compare\data\corpus\tech.txt
d:\vscode\rime_projs\rime-schema-compare\data\corpus\test.txt
```

### 训练数据目录

统一训练数据目录：

```text
E:\训练bert\data
```

当前回归质量分数据：

```text
E:\训练bert\data\quality_rime_frost_top10_50k_var2_5.train.jsonl
E:\训练bert\data\quality_rime_frost_top10_50k_var2_5.valid.jsonl
E:\训练bert\data\quality_rime_frost_top10_50k_var2_5.test.jsonl
E:\训练bert\data\quality_rime_frost_top10_50k_var2_5.all.jsonl
E:\训练bert\data\quality_rime_frost_top10_50k_var2_5.summary.json
```

生成参数：

```text
source: d:\vscode\rime-frost\cn_dicts_dazhu\zhihu_deal0.split_hanzi.txt
skip: 70000
limit: 50000
top_n: 10
candidate_count: 每条随机 2/3/4/5 个候选
quality: 1 - edit_distance(candidate, gold) / max(len(candidate), len(gold))
```

### 底座模型

Hugging Face 模型名：

```text
uer/chinese_roberta_L-2_H-128
```

本地缓存路径：

```text
d:\vscode\moqi-input-method-projs\moqi-im-windows\.tools\hf_models\uer_chinese_roberta_L-2_H-128
```

### 已训练模型目录

统一模型目录：

```text
E:\训练bert\models
```

当前回归质量分模型：

```text
E:\训练bert\models\quality_regression_l2_1epoch
E:\训练bert\models\quality_regression_l2_1epoch\best_encoder
E:\训练bert\models\quality_regression_l2_1epoch\best_scorer.pt
E:\训练bert\models\quality_regression_l2_1epoch\metrics.json
```

历史强监督模型：

```text
E:\训练bert\models\strong_top20_append_gold_randompos_l2_5epoch
E:\训练bert\models\strong_top1_vs_gold_randompos_l2_5epoch
E:\训练bert\models\strong_top20_append_gold_l2_5epoch
E:\训练bert\models\strong_top1_vs_gold_l2_5epoch
```

历史 listwise / hard negative 模型：

```text
E:\训练bert\models\listwise_rank_syllable_l2_top1w2_10epoch
E:\训练bert\models\listwise_rank_syllable_l2_top1w2_hardneg_10epoch
E:\训练bert\models\listwise_rank_syllable_l2_top20_50k_3epoch
```

### 评估结果目录

统一评估目录：

```text
E:\训练bert\eval
```

原始 Top3 + 强监督模型评估结果：

```text
E:\训练bert\eval\original_top3_strong\small_corpus_top3_rime249.json
E:\训练bert\eval\original_top3_strong\small_corpus_top3_rime249.csv
E:\训练bert\eval\original_top3_strong\holdout_50001_70000_top3_rime249.json
E:\训练bert\eval\original_top3_strong\holdout_50001_70000_top3_rime249.csv
E:\训练bert\eval\original_top3_strong\holdout_250001_270000_top3_rime249.json
E:\训练bert\eval\original_top3_strong\holdout_250001_270000_top3_rime249.csv
```

人工插入 gold / 召回增强实验结果：

```text
E:\训练bert\eval\top3_injection
E:\训练bert\eval\recall_injection
E:\训练bert\eval\oracle
```

TopN 召回诊断结果：

```text
E:\训练bert\eval\topn_recall
```

### 核心脚本

清洗和拼音工具：

```text
d:\vscode\rime_projs\rime-schema-compare\src\rime_schema_compare\text_pipeline.py
```

Rime 解码封装：

```text
d:\vscode\rime_projs\rime-schema-compare\src\rime_schema_compare\rime_runner.py
d:\vscode\rime_projs\rime-schema-compare\src\rime_schema_compare\call_librime.py
```

通用 benchmark：

```text
d:\vscode\rime_projs\rime-schema-compare\scripts\benchmark_sentences.py
d:\vscode\rime_projs\rime-schema-compare\scripts\run_test_top3.ps1
```

回归质量分数据生成：

```text
d:\vscode\rime_projs\rime-schema-compare\scripts\generate_quality_regression_dataset.py
```

回归质量分训练：

```text
d:\vscode\rime_projs\rime-schema-compare\scripts\train_quality_regression_reranker.py
```

TopN 真实候选评估：

```text
d:\vscode\rime_projs\rime-schema-compare\scripts\evaluate_reranker_topn.py
```

JSONL listwise/回归候选评估：

```text
d:\vscode\rime_projs\rime-schema-compare\scripts\evaluate_listwise_reranker_jsonl.py
```

rank bias 扫描：

```text
d:\vscode\rime_projs\rime-schema-compare\scripts\sweep_rank_bias.py
```

实验辅助脚本：

```text
d:\vscode\rime_projs\rime-schema-compare\scripts\diagnose_rime_topn_recall.py
d:\vscode\rime_projs\rime-schema-compare\scripts\build_strong_supervised_listwise.py
d:\vscode\rime_projs\rime-schema-compare\scripts\build_oracle_listwise_from_recall.py
d:\vscode\rime_projs\rime-schema-compare\scripts\build_recall_injection_listwise.py
d:\vscode\rime_projs\rime-schema-compare\scripts\build_top3_rank2_injection_listwise.py
d:\vscode\rime_projs\rime-schema-compare\scripts\extract_hard_negative_listwise.py
d:\vscode\rime_projs\rime-schema-compare\scripts\split_listwise_dataset.py
```

### 常用命令模板

生成回归质量分数据：

```powershell
python scripts/generate_quality_regression_dataset.py `
  --source "d:\vscode\rime-frost\cn_dicts_dazhu\zhihu_deal0.split_hanzi.txt" `
  --out-dir "E:\训练bert\data" `
  --name quality_rime_frost_top10_50k_var2_5 `
  --vendor rime_frost `
  --rime-dll "lib\rime-24986039806.dll" `
  --skip 70000 `
  --limit 50000 `
  --top-n 10 `
  --progress-every 5000
```

训练 1 epoch 回归模型：

```powershell
python scripts/train_quality_regression_reranker.py `
  --model "d:/vscode/moqi-input-method-projs/moqi-im-windows/.tools/hf_models/uer_chinese_roberta_L-2_H-128" `
  --train "E:\训练bert\data\quality_rime_frost_top10_50k_var2_5.train.jsonl" `
  --valid "E:\训练bert\data\quality_rime_frost_top10_50k_var2_5.valid.jsonl" `
  --test "E:\训练bert\data\quality_rime_frost_top10_50k_var2_5.test.jsonl" `
  --out "E:\训练bert\models\quality_regression_l2_1epoch" `
  --epochs 1 `
  --batch-size 64 `
  --max-length 128
```

评估原始 Rime Top3 + 模型：

```powershell
python scripts/evaluate_reranker_topn.py `
  --vendor rime_frost `
  --rime-dll "lib\rime-24986039806.dll" `
  --corpus "E:\训练bert\data\corpus\zhihu_deal0.split_hanzi.holdout_50001_70000.txt" `
  --top-n 3 `
  --encoder "E:\训练bert\models\quality_regression_l2_1epoch\best_encoder" `
  --scorer "E:\训练bert\models\quality_regression_l2_1epoch\best_scorer.pt" `
  --out-csv "E:\训练bert\eval\quality_regression_l2_1epoch\holdout_50001_70000_top3.csv" `
  --out-json "E:\训练bert\eval\quality_regression_l2_1epoch\holdout_50001_70000_top3.json" `
  --batch-size 256 `
  --max-length 128 `
  --min-rerank-candidates 2 `
  --rank-input
```

