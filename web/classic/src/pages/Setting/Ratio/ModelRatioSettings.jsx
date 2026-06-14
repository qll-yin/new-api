/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import React, { useEffect, useRef, useState } from 'react';
import {
  Button,
  Col,
  Form,
  Popconfirm,
  Row,
  Space,
  Spin,
} from '@douyinfe/semi-ui';
import {
  API,
  compareObjects,
  showError,
  showSuccess,
  showWarning,
  verifyJSON,
} from '../../../helpers';
import { useTranslation } from 'react-i18next';

const jsonRule = {
  validator: (rule, value) => verifyJSON(value),
  message: '不是合法的 JSON 字符串',
};

export default function ModelRatioSettings(props) {
  const [loading, setLoading] = useState(false);
  const [inputs, setInputs] = useState({
    ModelPrice: '',
    VideoModelConfig: '',
    ModelRatio: '',
    CacheRatio: '',
    CreateCacheRatio: '',
    CompletionRatio: '',
    ImageRatio: '',
    AudioRatio: '',
    AudioCompletionRatio: '',
    ExposeRatioEnabled: false,
  });
  const [inputsRow, setInputsRow] = useState(inputs);
  const refForm = useRef();
  const { t } = useTranslation();

  async function onSubmit() {
    try {
      await refForm.current.validate();
      const updateArray = compareObjects(inputs, inputsRow);
      if (!updateArray.length) {
        return showWarning(t('你似乎并没有修改什么'));
      }

      const requestQueue = updateArray.map((item) => {
        const value =
          typeof inputs[item.key] === 'boolean'
            ? String(inputs[item.key])
            : inputs[item.key];
        return API.put('/api/option/', { key: item.key, value });
      });

      setLoading(true);
      const res = await Promise.all(requestQueue);
      if (res.includes(undefined)) {
        return showError(
          requestQueue.length > 1
            ? t('部分保存失败，请重试')
            : t('保存失败'),
        );
      }

      for (let i = 0; i < res.length; i++) {
        if (!res[i].data.success) {
          return showError(res[i].data.message);
        }
      }

      showSuccess(t('保存成功'));
      props.refresh();
    } catch (error) {
      console.error(error);
      showError(t('请检查输入'));
    } finally {
      setLoading(false);
    }
  }

  async function resetModelRatio() {
    try {
      const res = await API.post('/api/option/rest_model_ratio');
      if (res.data.success) {
        showSuccess(res.data.message);
        props.refresh();
      } else {
        showError(res.data.message);
      }
    } catch (error) {
      showError(error);
    }
  }

  useEffect(() => {
    const currentInputs = {};
    for (const key in props.options) {
      if (Object.keys(inputs).includes(key)) {
        currentInputs[key] = props.options[key];
      }
    }
    setInputs(currentInputs);
    setInputsRow(structuredClone(currentInputs));
    refForm.current?.setValues(currentInputs);
  }, [props.options]);

  const onFieldChange = (key, value) => {
    setInputs((prev) => ({ ...prev, [key]: value }));
  };

  return (
    <Spin spinning={loading}>
      <Form
        values={inputs}
        getFormApi={(formAPI) => (refForm.current = formAPI)}
        style={{ marginBottom: 15 }}
      >
        <Row gutter={16}>
          <Col xs={24} sm={16}>
            <Form.TextArea
              label={t('模型固定价格')}
              extraText={t('一次调用消耗多少美元，优先级高于模型倍率')}
              placeholder={t(
                '一个 JSON 文本，键为模型名称，值为固定价格，例如：{"gpt-4-gizmo-*":0.1}',
              )}
              field='ModelPrice'
              autosize={{ minRows: 6, maxRows: 12 }}
              trigger='blur'
              stopValidateWithError
              rules={[jsonRule]}
              onChange={(value) => onFieldChange('ModelPrice', value)}
            />
          </Col>
        </Row>

        <Row gutter={16}>
          <Col xs={24} sm={16}>
            <Form.TextArea
              label={t('视频计费配置')}
              extraText={t(
                'JSON 文本，键为模型名称，值为视频基准分辨率和各分辨率倍率',
              )}
              placeholder={t(
                '例如：{"happyhorse-1.0-t2v":{"base_resolution":"720P","resolution_multipliers":{"1080P":1.7142857143}}}',
              )}
              field='VideoModelConfig'
              autosize={{ minRows: 6, maxRows: 12 }}
              trigger='blur'
              stopValidateWithError
              rules={[jsonRule]}
              onChange={(value) => onFieldChange('VideoModelConfig', value)}
            />
          </Col>
        </Row>

        <Row gutter={16}>
          <Col xs={24} sm={16}>
            <Form.TextArea
              label={t('模型倍率')}
              placeholder={t('一个 JSON 文本，键为模型名称，值为倍率')}
              field='ModelRatio'
              autosize={{ minRows: 6, maxRows: 12 }}
              trigger='blur'
              stopValidateWithError
              rules={[jsonRule]}
              onChange={(value) => onFieldChange('ModelRatio', value)}
            />
          </Col>
        </Row>

        <Row gutter={16}>
          <Col xs={24} sm={16}>
            <Form.TextArea
              label={t('提示缓存倍率')}
              placeholder={t('一个 JSON 文本，键为模型名称，值为倍率')}
              field='CacheRatio'
              autosize={{ minRows: 6, maxRows: 12 }}
              trigger='blur'
              stopValidateWithError
              rules={[jsonRule]}
              onChange={(value) => onFieldChange('CacheRatio', value)}
            />
          </Col>
        </Row>

        <Row gutter={16}>
          <Col xs={24} sm={16}>
            <Form.TextArea
              label={t('缓存创建倍率')}
              extraText={t(
                '默认用于 5m 缓存创建倍率，1h 缓存创建倍率由后端按固定乘法自动计算',
              )}
              placeholder={t('一个 JSON 文本，键为模型名称，值为倍率')}
              field='CreateCacheRatio'
              autosize={{ minRows: 6, maxRows: 12 }}
              trigger='blur'
              stopValidateWithError
              rules={[jsonRule]}
              onChange={(value) => onFieldChange('CreateCacheRatio', value)}
            />
          </Col>
        </Row>

        <Row gutter={16}>
          <Col xs={24} sm={16}>
            <Form.TextArea
              label={t('模型补全倍率（仅对自定义模型有效）')}
              extraText={t('仅对自定义模型有效')}
              placeholder={t('一个 JSON 文本，键为模型名称，值为倍率')}
              field='CompletionRatio'
              autosize={{ minRows: 6, maxRows: 12 }}
              trigger='blur'
              stopValidateWithError
              rules={[jsonRule]}
              onChange={(value) => onFieldChange('CompletionRatio', value)}
            />
          </Col>
        </Row>

        <Row gutter={16}>
          <Col xs={24} sm={16}>
            <Form.TextArea
              label={t('图片输入倍率（仅部分模型支持该计费）')}
              extraText={t('图片输入相关倍率配置，键为模型名称，值为倍率')}
              placeholder={t('例如：{"gpt-image-1":2}')}
              field='ImageRatio'
              autosize={{ minRows: 6, maxRows: 12 }}
              trigger='blur'
              stopValidateWithError
              rules={[jsonRule]}
              onChange={(value) => onFieldChange('ImageRatio', value)}
            />
          </Col>
        </Row>

        <Row gutter={16}>
          <Col xs={24} sm={16}>
            <Form.TextArea
              label={t('音频倍率（仅部分模型支持该计费）')}
              extraText={t('音频输入相关倍率配置，键为模型名称，值为倍率')}
              placeholder={t('例如：{"gpt-4o-audio-preview":16}')}
              field='AudioRatio'
              autosize={{ minRows: 6, maxRows: 12 }}
              trigger='blur'
              stopValidateWithError
              rules={[jsonRule]}
              onChange={(value) => onFieldChange('AudioRatio', value)}
            />
          </Col>
        </Row>

        <Row gutter={16}>
          <Col xs={24} sm={16}>
            <Form.TextArea
              label={t('音频补全倍率（仅部分模型支持该计费）')}
              extraText={t('音频输出补全相关倍率配置，键为模型名称，值为倍率')}
              placeholder={t('例如：{"gpt-4o-realtime":2}')}
              field='AudioCompletionRatio'
              autosize={{ minRows: 6, maxRows: 12 }}
              trigger='blur'
              stopValidateWithError
              rules={[jsonRule]}
              onChange={(value) =>
                onFieldChange('AudioCompletionRatio', value)
              }
            />
          </Col>
        </Row>

        <Row gutter={16}>
          <Col span={16}>
            <Form.Switch
              label={t('暴露倍率接口')}
              field='ExposeRatioEnabled'
              onChange={(value) => onFieldChange('ExposeRatioEnabled', value)}
            />
          </Col>
        </Row>
      </Form>

      <Space>
        <Button onClick={onSubmit}>{t('保存模型倍率设置')}</Button>
        <Popconfirm
          title={t('确定重置模型倍率吗？')}
          content={t('此修改将不可逆')}
          okType='danger'
          position='top'
          onConfirm={resetModelRatio}
        >
          <Button type='danger'>{t('重置模型倍率')}</Button>
        </Popconfirm>
      </Space>
    </Spin>
  );
}
