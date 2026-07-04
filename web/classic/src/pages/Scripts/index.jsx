import React, { useEffect, useState } from 'react';
import { Button, Input, Modal, TabPane, Tabs, TextArea, Toast } from '@douyinfe/semi-ui';
import { API } from '../../helpers/api';

const emptyForm = { id: 0, title: '', description: '', draft_code: '' };

function formatTime(value) {
  if (!value) return '-';
  return new Date(value * 1000).toLocaleString();
}

function Scripts() {
  const [square, setSquare] = useState([]);
  const [mine, setMine] = useState([]);
  const [preview, setPreview] = useState(null);
  const [editing, setEditing] = useState(emptyForm);
  const [editorOpen, setEditorOpen] = useState(false);

  async function loadAll() {
    const [squareRes, mineRes] = await Promise.all([
      API.get('/api/scripts/square', { params: { limit: 100 } }),
      API.get('/api/scripts/mine'),
    ]);
    if (squareRes.data.success) setSquare(squareRes.data.data.items || []);
    if (mineRes.data.success) setMine(mineRes.data.data || []);
  }

  useEffect(() => {
    loadAll().catch((err) => Toast.error(String(err.message || err)));
  }, []);

  async function viewSquare(id) {
    const res = await API.get(`/api/scripts/square/${id}`);
    if (res.data.success) setPreview(res.data.data);
  }

  async function editMine(id) {
    const res = await API.get(`/api/scripts/mine/${id}`);
    if (!res.data.success) return;
    const script = res.data.data;
    setEditing({
      id: script.id,
      title: script.title || '',
      description: script.description || '',
      draft_code: script.draft_code || '',
    });
    setEditorOpen(true);
  }

  async function saveDraft() {
    const payload = {
      title: editing.title,
      description: editing.description,
      code: editing.draft_code,
    };
    if (editing.id) await API.put(`/api/scripts/mine/${editing.id}`, payload);
    else await API.post('/api/scripts/mine', payload);
    Toast.success('草稿已保存');
    setEditorOpen(false);
    setEditing(emptyForm);
    await loadAll();
  }

  async function publishScript(id) {
    await API.post(`/api/scripts/mine/${id}/publish`);
    Toast.success('脚本已发布');
    await loadAll();
  }

  async function deleteScript(id) {
    if (!window.confirm(`确认删除脚本 #${id}？`)) return;
    await API.delete(`/api/scripts/mine/${id}`);
    Toast.success('脚本已删除');
    await loadAll();
  }

  const renderRows = (items, mineMode = false) =>
    items.map((script) => (
      <tr key={script.id} className='border-b border-gray-200'>
        <td className='p-2'>{script.id}</td>
        <td className='p-2'>{script.title}</td>
        <td className='p-2'>{script.description}</td>
        {mineMode && <td className='p-2'>{script.published ? '已发布' : '草稿'}</td>}
        <td className='p-2'>{formatTime(script.created_at)}</td>
        <td className='p-2'>{formatTime(script.updated_at)}</td>
        <td className='p-2 text-right'>
          {mineMode ? (
            <div className='flex justify-end gap-2'>
              <Button size='small' onClick={() => editMine(script.id)}>编辑</Button>
              <Button size='small' theme='solid' onClick={() => publishScript(script.id)}>发布</Button>
              <Button size='small' type='danger' onClick={() => deleteScript(script.id)}>删除</Button>
            </div>
          ) : (
            <Button size='small' onClick={() => viewSquare(script.id)}>查看代码</Button>
          )}
        </td>
      </tr>
    ));

  return (
    <div className='mt-[60px] px-4'>
      <div className='mb-3 flex items-center justify-between'>
        <h2 className='text-xl font-semibold'>脚本广场</h2>
        <div className='flex gap-2'>
          <Button onClick={loadAll}>刷新</Button>
          <Button theme='solid' onClick={() => { setEditing(emptyForm); setEditorOpen(true); }}>
            创建脚本
          </Button>
        </div>
      </div>
      <Tabs type='line'>
        <TabPane tab='广场' itemKey='square'>
          <table className='w-full text-sm'>
            <thead>
              <tr className='border-b text-left'>
                <th className='p-2'>ID</th>
                <th className='p-2'>Title</th>
                <th className='p-2'>Description</th>
                <th className='p-2'>创建时间</th>
                <th className='p-2'>修改时间</th>
                <th className='p-2 text-right'>操作</th>
              </tr>
            </thead>
            <tbody>{renderRows(square)}</tbody>
          </table>
        </TabPane>
        <TabPane tab='我的脚本' itemKey='mine'>
          <table className='w-full text-sm'>
            <thead>
              <tr className='border-b text-left'>
                <th className='p-2'>ID</th>
                <th className='p-2'>Title</th>
                <th className='p-2'>Description</th>
                <th className='p-2'>状态</th>
                <th className='p-2'>创建时间</th>
                <th className='p-2'>修改时间</th>
                <th className='p-2 text-right'>操作</th>
              </tr>
            </thead>
            <tbody>{renderRows(mine, true)}</tbody>
          </table>
        </TabPane>
      </Tabs>

      <Modal title={preview?.title || '脚本代码'} visible={!!preview} onCancel={() => setPreview(null)} footer={null} width={820}>
        <p>{preview?.description}</p>
        <pre className='max-h-[520px] overflow-auto rounded border p-3 text-xs whitespace-pre-wrap'>
          {(preview?.code_preview || '') + (preview?.preview_truncated ? '\n\n/* preview truncated */' : '')}
        </pre>
      </Modal>

      <Modal title={editing.id ? `编辑脚本 #${editing.id}` : '创建脚本'} visible={editorOpen} onCancel={() => setEditorOpen(false)} onOk={saveDraft} width={900} okText='保存草稿'>
        <div className='flex flex-col gap-3'>
          <Input value={editing.title} placeholder='Title' onChange={(value) => setEditing((prev) => ({ ...prev, title: value }))} />
          <Input value={editing.description} placeholder='Description' onChange={(value) => setEditing((prev) => ({ ...prev, description: value }))} />
          <TextArea value={editing.draft_code} autosize={{ minRows: 16, maxRows: 24 }} placeholder='JavaScript code' onChange={(value) => setEditing((prev) => ({ ...prev, draft_code: value }))} />
        </div>
      </Modal>
    </div>
  );
}

export default Scripts;
