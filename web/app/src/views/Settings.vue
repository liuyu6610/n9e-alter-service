<template>
  <el-row :gutter="16">
    <el-col :span="24">
      <el-alert v-if="error" type="error" :closable="false" show-icon :title="error" />
      <el-alert
        v-else
        type="info"
        :closable="false"
        show-icon
        :title="`当前规则：exists=${exists} hash=${hash || '-'}`"
      />
    </el-col>

    <el-col :span="24" style="margin-top: 12px">
      <el-card shadow="never">
        <template #header>
          <div style="display: flex; justify-content: space-between; align-items: center">
            <div>API Token</div>
            <el-tag :type="apiTokenMasked ? 'success' : 'info'">{{ apiTokenMasked ? `已设置(${apiTokenMasked})` : '未设置' }}</el-tag>
          </div>
        </template>

        <el-form label-width="110px">
          <el-form-item label="api_token">
            <el-input v-model="apiTokenInput" type="password" show-password placeholder="用于访问 /api/v1/*（Bearer）" />
          </el-form-item>
          <el-form-item>
            <el-button type="primary" @click="saveApiToken">保存到浏览器</el-button>
            <el-button @click="clearApiToken">清除</el-button>
          </el-form-item>
        </el-form>
      </el-card>
    </el-col>

    <el-col :span="8" style="margin-top: 12px">
      <el-card>
        <template #header>
          <div style="display: flex; justify-content: space-between; align-items: center">
            <div>版本</div>
            <el-button :loading="versionsLoading" @click="loadVersions">刷新</el-button>
          </div>
        </template>

        <div v-if="versions.length === 0" style="color: #909399">暂无版本</div>
        <el-scrollbar height="520px">
          <el-menu :default-active="selectedVersion" @select="onSelectVersion">
            <el-menu-item v-for="v in versions" :key="v" :index="v">
              <span style="font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace">
                {{ v }}
              </span>
            </el-menu-item>
          </el-menu>
        </el-scrollbar>
      </el-card>
    </el-col>

    <el-col :span="16" style="margin-top: 12px">
      <el-card>
        <template #header>
          <div style="display: flex; justify-content: space-between; align-items: center">
            <div>运行配置（RuleSet）</div>
            <div style="display: flex; gap: 8px">
              <el-button :loading="loading" @click="reload">刷新</el-button>
              <el-button :loading="auditsLoading" @click="loadAudits">审计</el-button>
              <el-button type="warning" :disabled="!selectedVersion" :loading="rolling" @click="rollback">回滚到选中版本</el-button>
              <el-button :disabled="!formReady" @click="syncFormToJson">表单→JSON</el-button>
              <el-button :disabled="rulesJson.trim().length === 0" @click="syncJsonToForm">JSON→表单</el-button>
              <el-button type="primary" :loading="publishing" @click="publish">发布</el-button>
            </div>
          </div>
        </template>

        <el-form label-width="110px" style="margin-bottom: 12px">
          <el-form-item label="actor">
            <el-input v-model="actor" placeholder="可选" />
          </el-form-item>
          <el-form-item label="message">
            <el-input v-model="message" placeholder="可选" />
          </el-form-item>
        </el-form>

        <el-form v-if="formModel" ref="formRef" :model="formModel" :rules="rules" label-width="140px">
          <el-tabs v-model="activeTab">
            <el-tab-pane label="N9E" name="n9e">
              <el-form-item label="base_url" prop="n9e.base_url">
                <el-input v-model="formModel.n9e.base_url" placeholder="例如：https://n9e.example.com" />
              </el-form-item>
              <el-form-item label="api_path" prop="n9e.api_path">
                <el-input v-model="formModel.n9e.api_path" placeholder="例如：/api/n9e" />
              </el-form-item>
              <el-form-item label="user_token" prop="n9e.user_token">
                <el-input v-model="formModel.n9e.user_token" type="password" show-password />
              </el-form-item>
              <el-form-item label="authorization" prop="n9e.authorization">
                <el-input v-model="formModel.n9e.authorization" type="password" show-password placeholder="可选" />
              </el-form-item>
              <el-form-item label="timeout_seconds" prop="n9e.timeout_seconds">
                <el-input-number v-model="formModel.n9e.timeout_seconds" :min="0" :step="1" />
              </el-form-item>
              <el-form-item label="verify_tls" prop="n9e.verify_tls">
                <el-switch v-model="formModel.n9e.verify_tls" />
              </el-form-item>
            </el-tab-pane>

            <el-tab-pane label="Pull" name="pull">
              <el-form-item label="interval_seconds" prop="pull.interval_seconds">
                <el-input-number v-model="formModel.pull.interval_seconds" :min="1" :step="1" />
              </el-form-item>
              <el-form-item label="page_limit" prop="pull.page_limit">
                <el-input-number v-model="formModel.pull.page_limit" :min="1" :step="1" />
              </el-form-item>
              <el-form-item label="max_pages" prop="pull.max_pages">
                <el-input-number v-model="formModel.pull.max_pages" :min="0" :step="1" />
              </el-form-item>
              <el-form-item label="my_groups" prop="pull.my_groups">
                <el-switch v-model="formModel.pull.my_groups" />
              </el-form-item>
              <el-form-item label="hours" prop="pull.hours">
                <el-input-number v-model="formModel.pull.hours" :min="0" :step="1" />
              </el-form-item>
              <el-form-item label="query" prop="pull.query">
                <el-input v-model="formModel.pull.query" placeholder="可选" />
              </el-form-item>
            </el-tab-pane>

            <el-tab-pane label="Push" name="push">
              <el-form-item label="enabled" prop="push.enabled">
                <el-switch v-model="formModel.push.enabled" />
              </el-form-item>
              <el-form-item label="token" prop="push.token">
                <el-input v-model="formModel.push.token" type="password" show-password placeholder="开启 push 时建议配置" />
              </el-form-item>
              <el-form-item label="queue_size" prop="push.queue_size">
                <el-input-number v-model="formModel.push.queue_size" :min="0" :step="100" />
              </el-form-item>
              <el-form-item label="worker_count" prop="push.worker_count">
                <el-input-number v-model="formModel.push.worker_count" :min="0" :step="1" />
              </el-form-item>
              <el-form-item label="enqueue_timeout_milli" prop="push.enqueue_timeout_milli">
                <el-input-number v-model="formModel.push.enqueue_timeout_milli" :min="0" :step="100" />
              </el-form-item>
            </el-tab-pane>

            <el-tab-pane label="State" name="state">
              <el-form-item label="snapshot_file" prop="state.snapshot_file">
                <el-input v-model="formModel.state.snapshot_file" />
              </el-form-item>
              <el-form-item label="snapshot_interval_seconds" prop="state.snapshot_interval_seconds">
                <el-input-number v-model="formModel.state.snapshot_interval_seconds" :min="1" :step="1" />
              </el-form-item>
              <el-form-item label="retain_recovered_seconds" prop="state.retain_recovered_seconds">
                <el-input-number v-model="formModel.state.retain_recovered_seconds" :min="0" :step="60" />
              </el-form-item>
              <el-form-item label="recover_miss_count" prop="state.recover_miss_count">
                <el-input-number v-model="formModel.state.recover_miss_count" :min="0" :step="1" />
              </el-form-item>
            </el-tab-pane>

            <el-tab-pane label="Redis" name="redis">
              <el-form-item label="enabled" prop="state.redis.enabled">
                <el-switch v-model="formModel.state.redis.enabled" />
              </el-form-item>
              <el-form-item label="addr" prop="state.redis.addr">
                <el-input v-model="formModel.state.redis.addr" placeholder="例如：127.0.0.1:6379" />
              </el-form-item>
              <el-form-item label="password" prop="state.redis.password">
                <el-input v-model="formModel.state.redis.password" type="password" show-password placeholder="可选" />
              </el-form-item>
              <el-form-item label="db" prop="state.redis.db">
                <el-input-number v-model="formModel.state.redis.db" :min="0" :step="1" />
              </el-form-item>
              <el-form-item label="key_prefix" prop="state.redis.key_prefix">
                <el-input v-model="formModel.state.redis.key_prefix" placeholder="默认：n9e_alter" />
              </el-form-item>
              <el-form-item label="hot_ttl_seconds" prop="state.redis.hot_ttl_seconds">
                <el-input-number v-model="formModel.state.redis.hot_ttl_seconds" :min="0" :step="60" />
              </el-form-item>
              <el-form-item label="ttl_seconds" prop="state.redis.ttl_seconds">
                <el-input-number v-model="formModel.state.redis.ttl_seconds" :min="0" :step="60" />
              </el-form-item>
            </el-tab-pane>

            <el-tab-pane label="DingTalk" name="dingtalk">
              <el-form-item label="webhook" prop="dingtalk.webhook">
                <el-input v-model="formModel.dingtalk.webhook" placeholder="可选（全局默认）" />
              </el-form-item>
              <el-form-item label="secret" prop="dingtalk.secret">
                <el-input v-model="formModel.dingtalk.secret" type="password" show-password placeholder="可选" />
              </el-form-item>
              <el-form-item label="keyword" prop="dingtalk.keyword">
                <el-input v-model="formModel.dingtalk.keyword" placeholder="可选" />
              </el-form-item>
            </el-tab-pane>

            <el-tab-pane label="Routes" name="routes">
              <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px">
                <div style="font-weight: 600">路由</div>
                <el-button size="small" @click="addRoute">新增路由</el-button>
              </div>

              <el-table :data="formModel.routes" size="small" border style="width: 100%" row-key="name">
                <el-table-column type="expand">
                  <template #default="scope">
                    <div style="padding: 8px 8px 0 8px">
                      <el-form label-width="140px">
                        <el-form-item label="name">
                          <el-input v-model="scope.row.name" placeholder="route name" @blur="onRouteNameBlur(scope.$index)" />
                        </el-form-item>
                        <el-form-item label="enabled">
                          <el-switch v-model="scope.row.enabled" />
                        </el-form-item>
                        <el-form-item label="notify.enabled">
                          <el-switch v-model="scope.row.notify.enabled" />
                        </el-form-item>
                        <el-form-item label="notify.robot_id">
                          <el-input v-model="scope.row.notify.robot_id" placeholder="默认机器人ID（可选）" />
                        </el-form-item>
                        <el-form-item label="notify.observe_seconds">
                          <el-input-number v-model="scope.row.notify.observe_seconds" :min="0" :step="1" />
                        </el-form-item>
                        <el-form-item label="notify.repeat_interval_seconds">
                          <el-input-number v-model="scope.row.notify.repeat_interval_seconds" :min="0" :step="60" />
                        </el-form-item>
                        <el-form-item label="notify.send_recovered">
                          <el-switch v-model="scope.row.notify.send_recovered" />
                        </el-form-item>
                      </el-form>

                      <div style="margin: 8px 0; font-weight: 600">Webhook</div>
                      <el-form label-width="140px">
                        <el-form-item label="enabled">
                          <el-switch v-model="scope.row.notify.webhook.enabled" />
                        </el-form-item>
                        <el-form-item label="url">
                          <el-input v-model="scope.row.notify.webhook.url" placeholder="http(s)://..." />
                        </el-form-item>
                        <el-form-item label="timeout_seconds">
                          <el-input-number v-model="scope.row.notify.webhook.timeout_seconds" :min="1" :step="1" />
                        </el-form-item>
                        <el-form-item label="headers (json)">
                          <el-input
                            v-model="scope.row.notify.webhook.__headersJson"
                            type="textarea"
                            :autosize="{ minRows: 2, maxRows: 6 }"
                            placeholder='{"Authorization":"Bearer xxx"}'
                            @blur="onRouteWebhookHeadersBlur(scope.$index)"
                          />
                        </el-form-item>
                      </el-form>

                      <div style="margin: 8px 0; font-weight: 600; display: flex; justify-content: space-between; align-items: center">
                        <div>Escalations</div>
                        <el-button size="small" @click="addEscalation(scope.$index)">新增升级</el-button>
                      </div>
                      <el-table :data="scope.row.notify.escalations" size="small" border style="width: 100%">
                        <el-table-column label="#" width="48">
                          <template #default="s2">
                            <span>{{ s2.$index + 1 }}</span>
                          </template>
                        </el-table-column>
                        <el-table-column label="after_seconds" width="140">
                          <template #default="s2">
                            <el-input-number v-model="s2.row.after_seconds" :min="0" :step="60" style="width: 100%" />
                          </template>
                        </el-table-column>
                        <el-table-column label="repeat_interval_seconds" width="200">
                          <template #default="s2">
                            <el-input-number v-model="s2.row.repeat_interval_seconds" :min="0" :step="60" style="width: 100%" />
                          </template>
                        </el-table-column>
                        <el-table-column label="robot_ids" min-width="220">
                          <template #default="s2">
                            <el-input
                              v-model="s2.row.__robotIDsText"
                              placeholder="逗号分隔，例如: r1,r2"
                              @blur="onEscalationRobotIDsBlur(scope.$index, s2.$index)"
                            />
                          </template>
                        </el-table-column>
                        <el-table-column label="webhook.enabled" width="140">
                          <template #default="s2">
                            <el-switch v-model="s2.row.webhook.enabled" />
                          </template>
                        </el-table-column>
                        <el-table-column label="webhook.url" min-width="220">
                          <template #default="s2">
                            <el-input v-model="s2.row.webhook.url" placeholder="http(s)://..." />
                          </template>
                        </el-table-column>
                        <el-table-column label="webhook.headers (json)" min-width="220">
                          <template #default="s2">
                            <el-input
                              v-model="s2.row.webhook.__headersJson"
                              type="textarea"
                              :autosize="{ minRows: 2, maxRows: 6 }"
                              placeholder='{"Authorization":"Bearer xxx"}'
                              @blur="onEscalationWebhookHeadersBlur(scope.$index, s2.$index)"
                            />
                          </template>
                        </el-table-column>
                        <el-table-column label="op" width="80">
                          <template #default="s2">
                            <el-button size="small" type="danger" @click="removeEscalation(scope.$index, s2.$index)">删除</el-button>
                          </template>
                        </el-table-column>
                      </el-table>
                    </div>
                  </template>
                </el-table-column>

                <el-table-column label="enabled" width="90">
                  <template #default="scope">
                    <el-switch v-model="scope.row.enabled" />
                  </template>
                </el-table-column>
                <el-table-column prop="name" label="name" min-width="180" />
                <el-table-column label="match" min-width="260">
                  <template #default="scope">
                    <div style="font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace; font-size: 12px">
                      <div>group={{ (scope.row.match?.group_name_regex || '').trim() || '-' }}</div>
                      <div>rule={{ (scope.row.match?.rule_name_regex || '').trim() || '-' }}</div>
                    </div>
                  </template>
                </el-table-column>
                <el-table-column label="dedup" width="140">
                  <template #default="scope">
                    <el-tag type="info">{{ (scope.row.dedup?.mode || '').trim() || '-' }}</el-tag>
                  </template>
                </el-table-column>
                <el-table-column label="notify" width="100">
                  <template #default="scope">
                    <el-tag v-if="scope.row.notify?.enabled" type="success">on</el-tag>
                    <el-tag v-else type="info">off</el-tag>
                  </template>
                </el-table-column>
                <el-table-column label="webhook" width="120">
                  <template #default="scope">
                    <el-tag v-if="scope.row.notify?.webhook?.enabled" type="success">on</el-tag>
                    <el-tag v-else type="info">off</el-tag>
                  </template>
                </el-table-column>
                <el-table-column label="escalations" width="120">
                  <template #default="scope">
                    <el-tag v-if="(scope.row.notify?.escalations || []).length > 0" type="warning">{{ (scope.row.notify?.escalations || []).length }}</el-tag>
                    <el-tag v-else type="info">0</el-tag>
                  </template>
                </el-table-column>
                <el-table-column label="op" width="80">
                  <template #default="scope">
                    <el-button size="small" type="danger" @click="removeRoute(scope.$index)">删除</el-button>
                  </template>
                </el-table-column>
              </el-table>

              <div style="margin-top: 8px; color: #909399; font-size: 12px">
                Webhook headers / robot_ids 使用文本编辑：失焦时会解析并写回配置。
              </div>
            </el-tab-pane>

            <el-tab-pane label="Robots" name="robots">
              <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px">
                <div style="font-weight: 600">钉钉机器人（多群多 token）</div>
                <el-button size="small" @click="addRobot">新增机器人</el-button>
              </div>

              <el-table :data="formModel.robots" size="small" border style="width: 100%" row-key="id">
                <el-table-column label="#" width="48">
                  <template #default="scope"><span>{{ scope.$index + 1 }}</span></template>
                </el-table-column>
                <el-table-column label="id" min-width="160">
                  <template #default="scope">
                    <el-input v-model="scope.row.id" placeholder="robot id（唯一）" @blur="onRobotIDBlur(scope.$index)" />
                  </template>
                </el-table-column>
                <el-table-column label="webhook" min-width="260">
                  <template #default="scope">
                    <el-input v-model="scope.row.webhook" placeholder="https://oapi.dingtalk.com/robot/send?access_token=..." />
                  </template>
                </el-table-column>
                <el-table-column label="secret" min-width="180">
                  <template #default="scope">
                    <el-input v-model="scope.row.secret" type="password" show-password placeholder="可选" />
                  </template>
                </el-table-column>
                <el-table-column label="keyword" min-width="140">
                  <template #default="scope">
                    <el-input v-model="scope.row.keyword" placeholder="可选" />
                  </template>
                </el-table-column>
                <el-table-column label="fallback_robot_ids" min-width="220">
                  <template #default="scope">
                    <el-input v-model="scope.row.__fallbackText" placeholder="逗号分隔，例如: r1,r2" @blur="onRobotFallbackBlur(scope.$index)" />
                  </template>
                </el-table-column>
                <el-table-column label="op" width="80">
                  <template #default="scope">
                    <el-button size="small" type="danger" @click="removeRobot(scope.$index)">删除</el-button>
                  </template>
                </el-table-column>
              </el-table>

              <div style="margin-top: 8px; color: #909399; font-size: 12px">
                说明：Bindings 命中后会优先按绑定选择 robot_ids；否则使用 route.notify.robot_id；否则使用全局/route 的 dingtalk。
              </div>
            </el-tab-pane>

            <el-tab-pane label="Bindings" name="bindings">
              <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px">
                <div style="font-weight: 600">绑定规则（把不同告警/群发送到不同机器人）</div>
                <el-button size="small" @click="addBinding">新增绑定</el-button>
              </div>

              <el-table :data="formModel.bindings" size="small" border style="width: 100%" row-key="name">
                <el-table-column label="#" width="48">
                  <template #default="scope"><span>{{ scope.$index + 1 }}</span></template>
                </el-table-column>
                <el-table-column label="enabled" width="90">
                  <template #default="scope"><el-switch v-model="scope.row.enabled" /></template>
                </el-table-column>
                <el-table-column label="name" min-width="160">
                  <template #default="scope">
                    <el-input v-model="scope.row.name" placeholder="唯一标识" @blur="onBindingNameBlur(scope.$index)" />
                  </template>
                </el-table-column>
                <el-table-column label="priority" width="120">
                  <template #default="scope"><el-input-number v-model="scope.row.priority" :step="1" /></template>
                </el-table-column>
                <el-table-column label="route_name" min-width="140">
                  <template #default="scope"><el-input v-model="scope.row.route_name" placeholder="留空=所有路由" /></template>
                </el-table-column>
                <el-table-column label="group_id" width="120">
                  <template #default="scope"><el-input-number v-model="scope.row.group_id" :min="0" :step="1" style="width: 100%" /></template>
                </el-table-column>
                <el-table-column label="group_name_regex" min-width="160">
                  <template #default="scope"><el-input v-model="scope.row.group_name_regex" placeholder="可选" /></template>
                </el-table-column>
                <el-table-column label="rule_id" width="120">
                  <template #default="scope"><el-input-number v-model="scope.row.rule_id" :min="0" :step="1" style="width: 100%" /></template>
                </el-table-column>
                <el-table-column label="rule_name_regex" min-width="160">
                  <template #default="scope"><el-input v-model="scope.row.rule_name_regex" placeholder="可选" /></template>
                </el-table-column>
                <el-table-column label="robot_ids" min-width="200">
                  <template #default="scope">
                    <el-input v-model="scope.row.__robotIDsText" placeholder="逗号分隔，例如: r1,r2" @blur="onBindingRobotIDsBlur(scope.$index)" />
                  </template>
                </el-table-column>
                <el-table-column label="tags" width="120">
                  <template #default="scope">
                    <div style="display: flex; gap: 8px; align-items: center">
                      <el-tag type="info">{{ Object.keys(scope.row.tags || {}).length }}</el-tag>
                      <el-button size="small" @click="openBindingTagEditor(scope.$index, 'tags')">编辑</el-button>
                    </div>
                  </template>
                </el-table-column>
                <el-table-column label="tag_regex" width="140">
                  <template #default="scope">
                    <div style="display: flex; gap: 8px; align-items: center">
                      <el-tag type="info">{{ Object.keys(scope.row.tag_regex || {}).length }}</el-tag>
                      <el-button size="small" @click="openBindingTagEditor(scope.$index, 'tag_regex')">编辑</el-button>
                    </div>
                  </template>
                </el-table-column>
                <el-table-column label="op" width="80">
                  <template #default="scope"><el-button size="small" type="danger" @click="removeBinding(scope.$index)">删除</el-button></template>
                </el-table-column>
              </el-table>

              <div style="margin-top: 8px; color: #909399; font-size: 12px">
                提示：priority 越大越优先；绑定命中后会取 robot_ids（或 robot_id）并去重发送。
              </div>
            </el-tab-pane>

            <el-tab-pane label="Silences" name="silences">
              <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px">
                <div style="font-weight: 600">抑制规则</div>
                <el-button
                  size="small"
                  @click="
                    formModel.silences = [
                      ...(formModel.silences || []),
                      { name: '', enabled: true, route_name: '', tags: {}, tag_regex: {}, expires_at_unix: 0 },
                    ]
                  "
                >
                  新增
                </el-button>
              </div>

              <el-table :data="formModel.silences" size="small" border style="width: 100%">
                <el-table-column label="#" width="48">
                  <template #default="scope">
                    <span>{{ scope.$index + 1 }}</span>
                  </template>
                </el-table-column>

                <el-table-column label="enabled" width="90">
                  <template #default="scope">
                    <el-switch v-model="scope.row.enabled" />
                  </template>
                </el-table-column>

                <el-table-column label="name" min-width="160">
                  <template #default="scope">
                    <el-input v-model="scope.row.name" placeholder="唯一标识/描述" />
                  </template>
                </el-table-column>

                <el-table-column label="route_name" min-width="140">
                  <template #default="scope">
                    <el-input v-model="scope.row.route_name" placeholder="留空=所有路由" />
                  </template>
                </el-table-column>

                <el-table-column label="expires_at_unix" width="160">
                  <template #default="scope">
                    <el-input-number v-model="scope.row.expires_at_unix" :min="0" :step="60" style="width: 100%" />
                  </template>
                </el-table-column>

                <el-table-column label="tags (json)" min-width="220">
                  <template #default="scope">
                    <el-input
                      v-model="scope.row.__tagsJson"
                      type="textarea"
                      :autosize="{ minRows: 2, maxRows: 6 }"
                      placeholder='{"cluster":"prod"}'
                      @blur="onSilenceTagsBlur(scope.$index)"
                    />
                  </template>
                </el-table-column>

                <el-table-column label="tag_regex (json)" min-width="220">
                  <template #default="scope">
                    <el-input
                      v-model="scope.row.__tagRegexJson"
                      type="textarea"
                      :autosize="{ minRows: 2, maxRows: 6 }"
                      placeholder='{"app":"^api-.*"}'
                      @blur="onSilenceTagRegexBlur(scope.$index)"
                    />
                  </template>
                </el-table-column>

                <el-table-column label="op" width="80">
                  <template #default="scope">
                    <el-button
                      size="small"
                      type="danger"
                      @click="formModel.silences = (formModel.silences || []).filter((_: any, i: number) => i !== scope.$index)"
                    >
                      删除
                    </el-button>
                  </template>
                </el-table-column>
              </el-table>

              <div style="margin-top: 8px; color: #909399; font-size: 12px">
                tags/tag_regex 使用 JSON 文本编辑：失焦时会解析并写回配置。
              </div>
            </el-tab-pane>
          </el-tabs>
        </el-form>

        <el-dialog v-model="bindingTagEditor.open" :title="bindingTagEditor.title" width="860px">
          <el-alert v-if="bindingTagEditor.err" type="error" :closable="false" show-icon :title="bindingTagEditor.err" style="margin-bottom: 10px" />
          <el-form label-width="120px">
            <el-form-item label="快速添加">
              <div style="display: flex; gap: 8px; width: 100%">
                <el-input v-model="bindingTagEditor.quickKey" placeholder="key" style="width: 220px" />
                <el-input v-model="bindingTagEditor.quickVal" placeholder="value / regex" style="flex: 1" />
                <el-button type="primary" @click="bindingTagEditorQuickAdd">添加/覆盖</el-button>
              </div>
            </el-form-item>
            <el-form-item label="批量编辑(JSON)">
              <el-input v-model="bindingTagEditor.json" type="textarea" :autosize="{ minRows: 12, maxRows: 24 }" placeholder='{"cluster":"prod","app":"api"}' />
            </el-form-item>
          </el-form>

          <template #footer>
            <div style="display: flex; justify-content: space-between; width: 100%">
              <div style="display: flex; gap: 8px">
                <el-button @click="bindingTagEditorValidate">校验</el-button>
                <el-button @click="bindingTagEditorFormat">格式化</el-button>
                <el-button type="warning" @click="bindingTagEditorClear">清空</el-button>
              </div>
              <div style="display: flex; gap: 8px">
                <el-button @click="bindingTagEditor.open = false">取消</el-button>
                <el-button type="primary" @click="bindingTagEditorSave">保存</el-button>
              </div>
            </div>
          </template>
        </el-dialog>

        <el-collapse style="margin-top: 12px">
          <el-collapse-item name="json" title="高级：RuleSet JSON（导入/导出）">
            <el-input
              v-model="rulesJson"
              type="textarea"
              :autosize="{ minRows: 10, maxRows: 30 }"
              placeholder="rulesrepo.RuleSet JSON"
            />
          </el-collapse-item>
        </el-collapse>

        <div style="margin-top: 12px">
          <div style="font-weight: 600; margin-bottom: 8px">审计（最近 {{ audits.length }} 条）</div>
          <el-table :data="audits" size="small" border height="240">
            <el-table-column prop="at_unix" label="at" width="140" />
            <el-table-column prop="action" label="action" width="100" />
            <el-table-column prop="version" label="version" width="170" />
            <el-table-column prop="hash" label="hash" width="180" />
            <el-table-column prop="actor" label="actor" width="120" />
            <el-table-column prop="message" label="message" />
          </el-table>
        </div>
      </el-card>
    </el-col>
  </el-row>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'

import { getRulesCurrent, getRulesVersion, listRulesAudits, listRulesVersions, publishRules, rollbackRules } from '../api/client'
import type { AuditRecord, RuleSet } from '../api/types'

type FormInstance = any

function deepClone(v: any): any {
  return JSON.parse(JSON.stringify(v))
}

function safeParseObject(s: string): Record<string, string> {
  const v = (s || '').trim()
  if (v.length === 0) {
    return {}
  }
  const obj = JSON.parse(v)
  if (!obj || typeof obj !== 'object' || Array.isArray(obj)) {
    throw new Error('must be json object')
  }
  const out: Record<string, string> = {}
  for (const k of Object.keys(obj)) {
    const vv = (obj as any)[k]
    out[String(k)] = vv == null ? '' : String(vv)
  }
  return out
}

function safeParseStringArray(s: string): string[] {
  const v = (s || '').trim()
  if (v.length === 0) {
    return []
  }
  return v
    .split(',')
    .map((x) => x.trim())
    .filter((x) => x.length > 0)
}

function sanitizeRuleSetForOutput(rs: RuleSet): RuleSet {
  const out: any = deepClone(rs)

  if (Array.isArray(out.silences)) {
    out.silences = out.silences.map((s: any) => {
      const x = { ...s }
      delete x.__tagsJson
      delete x.__tagRegexJson
      return x
    })
  }

  if (Array.isArray(out.routes)) {
    out.routes = out.routes.map((r: any) => {
      const x = deepClone(r)
      if (x?.notify?.webhook) {
        delete x.notify.webhook.__headersJson
        if (x.notify.webhook.headers == null) {
          x.notify.webhook.headers = {}
        }
      }
      if (Array.isArray(x?.notify?.escalations)) {
        x.notify.escalations = x.notify.escalations.map((e: any) => {
          const ee = deepClone(e)
          delete ee.__robotIDsText
          if (ee?.webhook) {
            delete ee.webhook.__headersJson
            if (ee.webhook.headers == null) {
              ee.webhook.headers = {}
            }
          }
          return ee
        })
      }
      return x
    })
  }

  if (Array.isArray(out.robots)) {
    out.robots = out.robots.map((r: any) => {
      const x = { ...r }
      delete x.__fallbackText
      return x
    })
  }

  if (Array.isArray(out.bindings)) {
    out.bindings = out.bindings.map((b: any) => {
      const x = { ...b }
      delete x.__robotIDsText
      delete x.__tagsJson
      delete x.__tagRegexJson
      return x
    })
  }

  return out as RuleSet
}

function defaultRuleSet(): RuleSet {
  return {
    n9e: {
      base_url: '',
      api_path: '/api/n9e',
      user_token: '',
      authorization: '',
      timeout_seconds: 10,
      verify_tls: true,
    },
    pull: {
      interval_seconds: 30,
      page_limit: 200,
      max_pages: 0,
      my_groups: false,
      hours: 24,
      stime: 0,
      etime: 0,
      query: '',
      severity: '',
      prods: '',
      rule_prods: '',
      cate: '',
      rid: 0,
      event_ids: '',
    },
    push: {
      enabled: false,
      token: '',
      queue_size: 20000,
      worker_count: 8,
      enqueue_timeout_milli: 0,
    },
    state: {
      snapshot_file: 'data/state_snapshot.json',
      snapshot_interval_seconds: 30,
      retain_recovered_seconds: 0,
      recover_miss_count: 0,
      redis: {
        enabled: false,
        addr: '',
        password: '',
        db: 0,
        key_prefix: 'n9e_alter',
        hot_ttl_seconds: 86400,
        ttl_seconds: 0,
      },
    },
    silences: [],
    routes: [],
    robots: [],
    bindings: [],
    dingtalk: {
      webhook: '',
      secret: '',
      keyword: '',
    },
  }
}

const loading = ref(false)
const publishing = ref(false)
const error = ref('')

const exists = ref(false)
const hash = ref('')

const actor = ref('')
const message = ref('')

const rulesJson = ref('')

const activeTab = ref('n9e')
const formModel = ref(null as unknown as RuleSet | null)
const formRef = ref(null as unknown as FormInstance | null)
const formReady = ref(false)

const apiTokenInput = ref('')
const apiTokenMasked = ref('')

function maskToken(v: string) {
  const s = String(v || '').trim()
  if (!s) return ''
  if (s.length <= 6) return '***'
  return `${s.slice(0, 2)}***${s.slice(-2)}`
}

function loadApiToken() {
  try {
    const v = (typeof window !== 'undefined' ? window.localStorage.getItem('api_token') : '') || ''
    apiTokenInput.value = v
    apiTokenMasked.value = maskToken(v)
  } catch {
    apiTokenInput.value = ''
    apiTokenMasked.value = ''
  }
}

function saveApiToken() {
  try {
    const v = (apiTokenInput.value || '').trim()
    if (typeof window !== 'undefined') {
      if (v) {
        window.localStorage.setItem('api_token', v)
      } else {
        window.localStorage.removeItem('api_token')
      }
    }
    apiTokenMasked.value = maskToken(v)
    ElMessage.success('api_token 已保存')
  } catch (e: any) {
    ElMessage.error(e?.message || '保存失败')
  }
}

function clearApiToken() {
  apiTokenInput.value = ''
  try {
    if (typeof window !== 'undefined') {
      window.localStorage.removeItem('api_token')
    }
  } catch {
    // ignore
  }
  apiTokenMasked.value = ''
  ElMessage.success('已清除 api_token')
}

const rules = {
  'n9e.timeout_seconds': [{ type: 'number', min: 0, message: 'timeout_seconds 需 >= 0', trigger: 'change' }],
  'pull.interval_seconds': [{ type: 'number', min: 1, message: 'interval_seconds 需 >= 1', trigger: 'change' }],
  'state.snapshot_interval_seconds': [{ type: 'number', min: 1, message: 'snapshot_interval_seconds 需 >= 1', trigger: 'change' }],
  'state.redis.addr': [
    {
      validator: (_: any, v: string, cb: (err?: Error) => void) => {
        if (!formModel.value) {
          cb()
          return
        }
        if (!formModel.value.state.redis.enabled) {
          cb()
          return
        }
        if ((v || '').trim().length === 0) {
          cb(new Error('Redis enabled 时 addr 必填'))
          return
        }
        cb()
      },
      trigger: 'blur',
    },
  ],
}

function setRuleSet(rs: RuleSet) {
  const model = deepClone(rs)

  if (Array.isArray(model.silences)) {
    model.silences = (model.silences as any[]).map((s: any) => {
      const tags = s?.tags && typeof s.tags === 'object' ? s.tags : {}
      const tagRegex = s?.tag_regex && typeof s.tag_regex === 'object' ? s.tag_regex : {}
      return {
        ...s,
        tags,
        tag_regex: tagRegex,
        __tagsJson: JSON.stringify(tags || {}, null, 0),
        __tagRegexJson: JSON.stringify(tagRegex || {}, null, 0),
      }
    })
  }

  if (Array.isArray(model.routes)) {
    model.routes = (model.routes as any[]).map((r: any) => {
      const nr = deepClone(r)
      if (!nr.notify) {
        nr.notify = {
          enabled: false,
          dingtalk: { webhook: '', secret: '', keyword: '' },
          webhook: { enabled: false, url: '', timeout_seconds: 5, headers: {} },
          robot_id: '',
          observe_seconds: 0,
          repeat_interval_seconds: 3600,
          send_recovered: true,
          escalations: [],
        }
      }
      if (!nr.notify.webhook) {
        nr.notify.webhook = { enabled: false, url: '', timeout_seconds: 5, headers: {} }
      }
      const h0 = nr.notify.webhook.headers && typeof nr.notify.webhook.headers === 'object' ? nr.notify.webhook.headers : {}
      nr.notify.webhook.headers = h0
      nr.notify.webhook.__headersJson = JSON.stringify(h0 || {}, null, 0)

      if (!Array.isArray(nr.notify.escalations)) {
        nr.notify.escalations = []
      }
      nr.notify.escalations = nr.notify.escalations.map((e: any) => {
        const ne = deepClone(e)
        if (!ne.webhook) {
          ne.webhook = { enabled: false, url: '', timeout_seconds: 5, headers: {} }
        }
        const h1 = ne.webhook.headers && typeof ne.webhook.headers === 'object' ? ne.webhook.headers : {}
        ne.webhook.headers = h1
        ne.webhook.__headersJson = JSON.stringify(h1 || {}, null, 0)

        const arr = Array.isArray(ne.robot_ids) ? ne.robot_ids : []
        ne.robot_ids = arr
        ne.__robotIDsText = (arr || []).join(',')
        return ne
      })

      return nr
    })
  }

  if (!Array.isArray((model as any).robots)) {
    ;(model as any).robots = []
  }
  ;(model as any).robots = ((model as any).robots || []).map((r: any) => {
    const nr = deepClone(r || {})
    if (!Array.isArray(nr.fallback_robot_ids)) {
      nr.fallback_robot_ids = []
    }
    nr.__fallbackText = (nr.fallback_robot_ids || []).join(',')
    return nr
  })

  if (!Array.isArray((model as any).bindings)) {
    ;(model as any).bindings = []
  }
  ;(model as any).bindings = ((model as any).bindings || []).map((b: any) => {
    const nb = deepClone(b || {})
    if (!Array.isArray(nb.robot_ids)) {
      nb.robot_ids = []
    }
    nb.__robotIDsText = (nb.robot_ids || []).join(',')
    const tags = nb?.tags && typeof nb.tags === 'object' ? nb.tags : {}
    const tagRegex = nb?.tag_regex && typeof nb.tag_regex === 'object' ? nb.tag_regex : {}
    nb.tags = tags
    nb.tag_regex = tagRegex
    nb.__tagsJson = JSON.stringify(tags || {}, null, 0)
    nb.__tagRegexJson = JSON.stringify(tagRegex || {}, null, 0)
    return nb
  })

  formModel.value = model
  formReady.value = true
  rulesJson.value = JSON.stringify(sanitizeRuleSetForOutput(model), null, 2)
}

function onSilenceTagsBlur(idx: number) {
  if (!formModel.value) {
    return
  }
  const items = (formModel.value as any).silences || []
  if (idx < 0 || idx >= items.length) {
    return
  }
  try {
    items[idx].tags = safeParseObject(items[idx].__tagsJson || '')
    items[idx].__tagsJson = JSON.stringify(items[idx].tags || {}, null, 0)
  } catch (e: any) {
    error.value = `silences[${idx}].tags: ${e?.message || String(e)}`
  }
}

function onSilenceTagRegexBlur(idx: number) {
  if (!formModel.value) {
    return
  }
  const items = (formModel.value as any).silences || []
  if (idx < 0 || idx >= items.length) {
    return
  }
  try {
    items[idx].tag_regex = safeParseObject(items[idx].__tagRegexJson || '')
    items[idx].__tagRegexJson = JSON.stringify(items[idx].tag_regex || {}, null, 0)
  } catch (e: any) {
    error.value = `silences[${idx}].tag_regex: ${e?.message || String(e)}`
  }
}

async function syncFormToJson() {
  if (!formModel.value) {
    return
  }
  rulesJson.value = JSON.stringify(sanitizeRuleSetForOutput(formModel.value), null, 2)
}

async function syncJsonToForm() {
  error.value = ''
  try {
    const parsed = JSON.parse(rulesJson.value || '{}') as RuleSet
    setRuleSet({ ...defaultRuleSet(), ...parsed } as RuleSet)
  } catch (e: any) {
    error.value = e?.message || String(e)
  }
}

const versionsLoading = ref(false)
const versions = ref([] as string[])
const selectedVersion = ref('')

const auditsLoading = ref(false)
const audits = ref([] as AuditRecord[])
const rolling = ref(false)

const bindingTagEditor = ref({
  open: false,
  idx: -1,
  field: 'tags' as 'tags' | 'tag_regex',
  title: 'Tags Editor',
  json: '',
  quickKey: '',
  quickVal: '',
  err: '',
})

async function reload() {
  loading.value = true
  error.value = ''
  try {
    const data = await getRulesCurrent()
    exists.value = data.exists
    hash.value = data.hash || ''
    setRuleSet(data.rules)
  } catch (e: any) {
    error.value = e?.message || String(e)
  } finally {
    loading.value = false
  }
}

async function loadVersions() {
  versionsLoading.value = true
  error.value = ''
  try {
    const data = await listRulesVersions()
    versions.value = data.items || []
  } catch (e: any) {
    error.value = e?.message || String(e)
  } finally {
    versionsLoading.value = false
  }
}

async function onSelectVersion(v: string) {
  selectedVersion.value = v
  loading.value = true
  error.value = ''
  try {
    const data = await getRulesVersion(v)
    setRuleSet(data.rules)
  } catch (e: any) {
    error.value = e?.message || String(e)
  } finally {
    loading.value = false
  }
}

async function loadAudits() {
  auditsLoading.value = true
  error.value = ''
  try {
    const data = await listRulesAudits(50)
    audits.value = data.items || []
  } catch (e: any) {
    error.value = e?.message || String(e)
  } finally {
    auditsLoading.value = false
  }
}

async function rollback() {
  if (!selectedVersion.value) {
    return
  }
  rolling.value = true
  error.value = ''
  try {
    await rollbackRules({ version: selectedVersion.value, message: message.value, actor: actor.value })
    await reload()
    await loadVersions()
    await loadAudits()
  } catch (e: any) {
    error.value = e?.message || String(e)
  } finally {
    rolling.value = false
  }
}

async function publish() {
  publishing.value = true
  error.value = ''
  try {
    if (!formModel.value) {
      throw new Error('表单未就绪')
    }
    validateRoutesUniqueOrThrow()
    validateRobotsUniqueOrThrow()
    validateBindingsUniqueOrThrow()
    if (formRef.value && typeof formRef.value.validate === 'function') {
      await formRef.value.validate()
    }
    await publishRules({ rules: sanitizeRuleSetForOutput(formModel.value), message: message.value, actor: actor.value })
    await reload()
    await loadVersions()
    await loadAudits()
  } catch (e: any) {
    error.value = e?.message || String(e)
  } finally {
    publishing.value = false
  }
}

onMounted(() => {
  loadApiToken()
  formModel.value = defaultRuleSet()
  formReady.value = true
  rulesJson.value = JSON.stringify(formModel.value, null, 2)
  reload()
  loadVersions()
  loadAudits()

  // Import binding draft generated from Preview
  try {
    const key = 'n9e_alter_binding_draft'
    const s = typeof window !== 'undefined' ? window.localStorage.getItem(key) : ''
    if (s) {
      const draft = JSON.parse(s)
      window.localStorage.removeItem(key)
      if (draft && typeof draft === 'object') {
        const cur = (formModel.value as any)
        if (cur) {
          if (!Array.isArray(cur.bindings)) {
            cur.bindings = []
          }
          // normalize helper fields
          if (!Array.isArray(draft.robot_ids)) {
            draft.robot_ids = []
          }
          draft.__robotIDsText = (draft.robot_ids || []).join(',')
          const tags = draft?.tags && typeof draft.tags === 'object' ? draft.tags : {}
          const tagRegex = draft?.tag_regex && typeof draft.tag_regex === 'object' ? draft.tag_regex : {}
          draft.tags = tags
          draft.tag_regex = tagRegex
          draft.__tagsJson = JSON.stringify(tags || {}, null, 0)
          draft.__tagRegexJson = JSON.stringify(tagRegex || {}, null, 0)
          cur.bindings = [...cur.bindings, draft]
          activeTab.value = 'bindings'
          // open editor for tag_regex if present, else tags
          const idx = cur.bindings.length - 1
          if (Object.keys(tagRegex || {}).length > 0) {
            openBindingTagEditor(idx, 'tag_regex')
          } else {
            openBindingTagEditor(idx, 'tags')
          }
        }
      }
    }
  } catch {
    // ignore
  }
})

function addRoute() {
  if (!formModel.value) {
    return
  }
  const now = Date.now()
  const existing = new Set<string>(((formModel.value as any).routes || []).map((r: any) => String(r?.name || '').trim()).filter((x: string) => x.length > 0))
  let base = `route_${now}`
  if (existing.has(base)) {
    let i = 1
    while (existing.has(`${base}_${i}`)) {
      i++
    }
    base = `${base}_${i}`
  }
  const r: any = {
    name: base,
    enabled: true,
    match: { group_name_regex: '.*', rule_name_regex: '.*', severity_in: null },
    dedup: {
      mode: 'n9e_hash',
      include_group_id: true,
      include_group_name: true,
      include_rule_id: true,
      include_rule_name: true,
      include_severity: true,
      include_entity: true,
      normalize_pod_name: false,
      rewrites: null,
    },
    processors: [],
    notify: {
      enabled: false,
      dingtalk: { webhook: '', secret: '', keyword: '' },
      webhook: { enabled: false, url: '', timeout_seconds: 5, headers: {}, __headersJson: '{}' },
      robot_id: '',
      observe_seconds: 0,
      repeat_interval_seconds: 3600,
      send_recovered: true,
      escalations: [],
    },
    daily_report: {
      enabled: false,
      cron: '0 18 * * *',
      title_prefix: 'N9E 告警日报',
      max_lines: 50,
      max_chars: 15000,
      clear_mode: 'reset_notified',
    },
  }
  ;(formModel.value as any).routes = [...((formModel.value as any).routes || []), r]
}

function addRobot() {
  if (!formModel.value) {
    return
  }
  const items = (formModel.value as any).robots || []
  const now = Date.now()
  const r: any = {
    id: `robot_${now}`,
    webhook: '',
    secret: '',
    keyword: '',
    fallback_robot_ids: [],
    __fallbackText: '',
  }
  ;(formModel.value as any).robots = [...items, r]
  try {
    validateRobotsUniqueOrThrow()
  } catch {
    // ignore
  }
}

function removeRobot(idx: number) {
  if (!formModel.value) {
    return
  }
  const items = (formModel.value as any).robots || []
  ;(formModel.value as any).robots = items.filter((_: any, i: number) => i !== idx)
}

function onRobotIDBlur(idx: number) {
  if (!formModel.value) {
    return
  }
  const items = (formModel.value as any).robots || []
  if (idx < 0 || idx >= items.length) {
    return
  }
  items[idx].id = String(items[idx].id || '').trim()
  try {
    validateRobotsUniqueOrThrow()
  } catch (e: any) {
    error.value = e?.message || String(e)
  }
}

function onRobotFallbackBlur(idx: number) {
  if (!formModel.value) {
    return
  }
  const items = (formModel.value as any).robots || []
  if (idx < 0 || idx >= items.length) {
    return
  }
  try {
    items[idx].fallback_robot_ids = safeParseStringArray(items[idx].__fallbackText || '')
    items[idx].__fallbackText = (items[idx].fallback_robot_ids || []).join(',')
  } catch (e: any) {
    error.value = `robots[${idx}].fallback_robot_ids: ${e?.message || String(e)}`
  }
}

function addBinding() {
  if (!formModel.value) {
    return
  }
  const items = (formModel.value as any).bindings || []
  const now = Date.now()
  const b: any = {
    name: `binding_${now}`,
    priority: 0,
    enabled: true,
    robot_id: '',
    robot_ids: [],
    group_id: 0,
    group_name_regex: '',
    rule_id: 0,
    rule_name_regex: '',
    route_name: '',
    tags: {},
    tag_regex: {},
    __robotIDsText: '',
    __tagsJson: '{}',
    __tagRegexJson: '{}',
  }
  ;(formModel.value as any).bindings = [...items, b]
  try {
    validateBindingsUniqueOrThrow()
  } catch {
    // ignore
  }
}

function removeBinding(idx: number) {
  if (!formModel.value) {
    return
  }
  const items = (formModel.value as any).bindings || []
  ;(formModel.value as any).bindings = items.filter((_: any, i: number) => i !== idx)
}

function onBindingNameBlur(idx: number) {
  if (!formModel.value) {
    return
  }
  const items = (formModel.value as any).bindings || []
  if (idx < 0 || idx >= items.length) {
    return
  }
  items[idx].name = String(items[idx].name || '').trim()
  try {
    validateBindingsUniqueOrThrow()
  } catch (e: any) {
    error.value = e?.message || String(e)
  }
}

function onBindingRobotIDsBlur(idx: number) {
  if (!formModel.value) {
    return
  }
  const items = (formModel.value as any).bindings || []
  if (idx < 0 || idx >= items.length) {
    return
  }
  try {
    items[idx].robot_ids = safeParseStringArray(items[idx].__robotIDsText || '')
    items[idx].__robotIDsText = (items[idx].robot_ids || []).join(',')
  } catch (e: any) {
    error.value = `bindings[${idx}].robot_ids: ${e?.message || String(e)}`
  }
}

function onBindingTagsBlur(idx: number) {
  if (!formModel.value) {
    return
  }
  const items = (formModel.value as any).bindings || []
  if (idx < 0 || idx >= items.length) {
    return
  }
  try {
    items[idx].tags = safeParseObject(items[idx].__tagsJson || '')
    items[idx].__tagsJson = JSON.stringify(items[idx].tags || {}, null, 0)
  } catch (e: any) {
    error.value = `bindings[${idx}].tags: ${e?.message || String(e)}`
  }
}

function onBindingTagRegexBlur(idx: number) {
  if (!formModel.value) {
    return
  }
  const items = (formModel.value as any).bindings || []
  if (idx < 0 || idx >= items.length) {
    return
  }
  try {
    items[idx].tag_regex = safeParseObject(items[idx].__tagRegexJson || '')
    items[idx].__tagRegexJson = JSON.stringify(items[idx].tag_regex || {}, null, 0)
  } catch (e: any) {
    error.value = `bindings[${idx}].tag_regex: ${e?.message || String(e)}`
  }
}

function openBindingTagEditor(idx: number, field: 'tags' | 'tag_regex') {
  if (!formModel.value) {
    return
  }
  const items = (formModel.value as any).bindings || []
  if (idx < 0 || idx >= items.length) {
    return
  }
  const it = items[idx] || {}
  const obj = field === 'tags' ? (it.tags || {}) : (it.tag_regex || {})
  bindingTagEditor.value.open = true
  bindingTagEditor.value.idx = idx
  bindingTagEditor.value.field = field
  bindingTagEditor.value.title = field === 'tags' ? '编辑 tags（精确匹配）' : '编辑 tag_regex（正则匹配）'
  bindingTagEditor.value.json = JSON.stringify(obj || {}, null, 2)
  bindingTagEditor.value.quickKey = ''
  bindingTagEditor.value.quickVal = ''
  bindingTagEditor.value.err = ''
}

function bindingTagEditorValidate() {
  bindingTagEditor.value.err = ''
  try {
    safeParseObject(bindingTagEditor.value.json || '')
    ElMessage.success('JSON 校验通过')
  } catch (e: any) {
    bindingTagEditor.value.err = e?.message || String(e)
  }
}

function bindingTagEditorFormat() {
  bindingTagEditor.value.err = ''
  try {
    const obj = safeParseObject(bindingTagEditor.value.json || '')
    bindingTagEditor.value.json = JSON.stringify(obj, null, 2)
  } catch (e: any) {
    bindingTagEditor.value.err = e?.message || String(e)
  }
}

function bindingTagEditorClear() {
  bindingTagEditor.value.err = ''
  bindingTagEditor.value.json = '{}'
  bindingTagEditor.value.quickKey = ''
  bindingTagEditor.value.quickVal = ''
}

function bindingTagEditorQuickAdd() {
  bindingTagEditor.value.err = ''
  const k = String(bindingTagEditor.value.quickKey || '').trim()
  const v = String(bindingTagEditor.value.quickVal || '').trim()
  if (!k) {
    bindingTagEditor.value.err = 'key 不能为空'
    return
  }
  try {
    const obj = safeParseObject(bindingTagEditor.value.json || '')
    obj[k] = v
    bindingTagEditor.value.json = JSON.stringify(obj, null, 2)
    bindingTagEditor.value.quickKey = ''
    bindingTagEditor.value.quickVal = ''
  } catch (e: any) {
    bindingTagEditor.value.err = e?.message || String(e)
  }
}

function bindingTagEditorSave() {
  if (!formModel.value) {
    return
  }
  const idx = bindingTagEditor.value.idx
  const field = bindingTagEditor.value.field
  const items = (formModel.value as any).bindings || []
  if (idx < 0 || idx >= items.length) {
    bindingTagEditor.value.err = 'binding index invalid'
    return
  }
  bindingTagEditor.value.err = ''
  try {
    const obj = safeParseObject(bindingTagEditor.value.json || '')
    if (field === 'tags') {
      items[idx].tags = obj
      items[idx].__tagsJson = JSON.stringify(obj || {}, null, 0)
    } else {
      items[idx].tag_regex = obj
      items[idx].__tagRegexJson = JSON.stringify(obj || {}, null, 0)
    }
    bindingTagEditor.value.open = false
    ElMessage.success('已保存')
  } catch (e: any) {
    bindingTagEditor.value.err = e?.message || String(e)
  }
}

function removeRoute(idx: number) {
  if (!formModel.value) {
    return
  }
  const items = (formModel.value as any).routes || []
  ;(formModel.value as any).routes = items.filter((_: any, i: number) => i !== idx)
  try {
    validateRoutesUniqueOrThrow()
  } catch {
    // ignore
  }
}

function onRouteNameBlur(routeIdx: number) {
  if (!formModel.value) {
    return
  }
  const routes = (formModel.value as any).routes || []
  if (routeIdx < 0 || routeIdx >= routes.length) {
    return
  }
  const r = routes[routeIdx]
  if (!r) {
    return
  }
  r.name = String(r.name || '').trim()
  try {
    validateRoutesUniqueOrThrow()
  } catch (e: any) {
    error.value = e?.message || String(e)
  }
}

function validateRoutesUniqueOrThrow() {
  if (!formModel.value) {
    return
  }
  const routes = (formModel.value as any).routes || []
  const seen = new Map<string, number>()
  for (let i = 0; i < routes.length; i++) {
    const name = String(routes[i]?.name || '').trim()
    if (name.length === 0) {
      continue
    }
    const prev = seen.get(name)
    if (prev != null) {
      throw new Error(`routes name 重复: '${name}' (index=${prev},${i})`)
    }
    seen.set(name, i)
  }
}

function validateRobotsUniqueOrThrow() {
  if (!formModel.value) {
    return
  }
  const robots = (formModel.value as any).robots || []
  const seen = new Map<string, number>()
  for (let i = 0; i < robots.length; i++) {
    const id = String(robots[i]?.id || '').trim()
    if (!id) {
      continue
    }
    const prev = seen.get(id)
    if (prev != null) {
      throw new Error(`robots id 重复: '${id}' (index=${prev},${i})`)
    }
    seen.set(id, i)
  }
}

function validateBindingsUniqueOrThrow() {
  if (!formModel.value) {
    return
  }
  const items = (formModel.value as any).bindings || []
  const seen = new Map<string, number>()
  for (let i = 0; i < items.length; i++) {
    const name = String(items[i]?.name || '').trim()
    if (!name) {
      continue
    }
    const prev = seen.get(name)
    if (prev != null) {
      throw new Error(`bindings name 重复: '${name}' (index=${prev},${i})`)
    }
    seen.set(name, i)
  }
}

function onRouteWebhookHeadersBlur(routeIdx: number) {
  if (!formModel.value) {
    return
  }
  const routes = (formModel.value as any).routes || []
  if (routeIdx < 0 || routeIdx >= routes.length) {
    return
  }
  try {
    const w = routes[routeIdx]?.notify?.webhook
    if (!w) {
      return
    }
    w.headers = safeParseObject(w.__headersJson || '')
    w.__headersJson = JSON.stringify(w.headers || {}, null, 0)
  } catch (e: any) {
    error.value = `routes[${routeIdx}].notify.webhook.headers: ${e?.message || String(e)}`
  }
}

function addEscalation(routeIdx: number) {
  if (!formModel.value) {
    return
  }
  const routes = (formModel.value as any).routes || []
  if (routeIdx < 0 || routeIdx >= routes.length) {
    return
  }
  const r = routes[routeIdx]
  if (!r.notify) {
    r.notify = {}
  }
  if (!Array.isArray(r.notify.escalations)) {
    r.notify.escalations = []
  }
  r.notify.escalations = [
    ...r.notify.escalations,
    {
      after_seconds: 300,
      repeat_interval_seconds: 3600,
      robot_ids: [],
      __robotIDsText: '',
      webhook: { enabled: false, url: '', timeout_seconds: 5, headers: {}, __headersJson: '{}' },
    },
  ]
}

function removeEscalation(routeIdx: number, escIdx: number) {
  if (!formModel.value) {
    return
  }
  const routes = (formModel.value as any).routes || []
  if (routeIdx < 0 || routeIdx >= routes.length) {
    return
  }
  const r = routes[routeIdx]
  const items = r?.notify?.escalations || []
  r.notify.escalations = items.filter((_: any, i: number) => i !== escIdx)
}

function onEscalationRobotIDsBlur(routeIdx: number, escIdx: number) {
  if (!formModel.value) {
    return
  }
  const routes = (formModel.value as any).routes || []
  if (routeIdx < 0 || routeIdx >= routes.length) {
    return
  }
  const r = routes[routeIdx]
  const es = r?.notify?.escalations || []
  if (escIdx < 0 || escIdx >= es.length) {
    return
  }
  try {
    es[escIdx].robot_ids = safeParseStringArray(es[escIdx].__robotIDsText || '')
    es[escIdx].__robotIDsText = (es[escIdx].robot_ids || []).join(',')
  } catch (e: any) {
    error.value = `routes[${routeIdx}].notify.escalations[${escIdx}].robot_ids: ${e?.message || String(e)}`
  }
}

function onEscalationWebhookHeadersBlur(routeIdx: number, escIdx: number) {
  if (!formModel.value) {
    return
  }
  const routes = (formModel.value as any).routes || []
  if (routeIdx < 0 || routeIdx >= routes.length) {
    return
  }
  const r = routes[routeIdx]
  const es = r?.notify?.escalations || []
  if (escIdx < 0 || escIdx >= es.length) {
    return
  }
  try {
    const w = es[escIdx]?.webhook
    if (!w) {
      return
    }
    w.headers = safeParseObject(w.__headersJson || '')
    w.__headersJson = JSON.stringify(w.headers || {}, null, 0)
  } catch (e: any) {
    error.value = `routes[${routeIdx}].notify.escalations[${escIdx}].webhook.headers: ${e?.message || String(e)}`
  }
}
</script>
